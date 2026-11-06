/** 统计接口返回的失败任务。只保留重试所需的字段。 */
export type DeadJob = {
  id: string;
  kind: string;
  last_error_summary: string;
  updated_at: string;
};

export type RetryFeedback = {
  requeued: string[];
  gone: string[];
  conflict: string[];
  failed: string[];
  /** 已重新入队或已确认不在失败列表的 id，刷新失败时不再用这些 id 重复请求。 */
  handledIDs: string[];
};

export type DeadRetryResult<TStats extends { dead: number }, TList> = {
  ignored: false;
  feedback: RetryFeedback;
  /** 统计刷新失败时为 null，不能据此把剩余数量当成 0。 */
  stats: TStats | null;
  list: TList | null;
  note: string;
  alert: boolean;
  needsReload: boolean;
};

/**
 * 同一次点击只放行一轮重试。
 * 按钮的 disabled 要等渲染，重复点击在进入网络请求前就被挡住。
 */
export type RetryGate = {
  enter: () => boolean;
  leave: () => void;
};

export function createRetryGate(): RetryGate {
  let running = false;
  return {
    enter() {
      if (running) return false;
      running = true;
      return true;
    },
    leave() {
      running = false;
    },
  };
}

export async function withRetryGate<T>(
  gate: RetryGate,
  task: () => Promise<T>,
): Promise<T | { ignored: true }> {
  if (!gate.enter()) return { ignored: true };
  try {
    return await task();
  } finally {
    gate.leave();
  }
}

/** 按契约读取 dead_jobs。缺字段或条目不完整直接失败，不用别的字段顶替。 */
export function parseDeadJobs(value: unknown): DeadJob[] {
  if (!Array.isArray(value)) throw new Error('记忆统计格式不正确');
  return value.map((item) => {
    if (typeof item !== 'object' || item == null || Array.isArray(item)) {
      throw new Error('记忆统计格式不正确');
    }
    const job = item as Record<string, unknown>;
    if (
      typeof job.id !== 'string' ||
      job.id === '' ||
      typeof job.kind !== 'string' ||
      typeof job.last_error_summary !== 'string' ||
      typeof job.updated_at !== 'string'
    ) {
      throw new Error('记忆统计格式不正确');
    }
    return {
      id: job.id,
      kind: job.kind,
      last_error_summary: job.last_error_summary,
      updated_at: job.updated_at,
    };
  });
}

/** 保住服务端给出的顺序，重复 id 只请求一次。 */
export function deadJobIDs(jobs: readonly { id: string }[]): string[] {
  const ids: string[] = [];
  const seen = new Set<string>();
  for (const job of jobs) {
    if (job.id === '' || seen.has(job.id)) continue;
    seen.add(job.id);
    ids.push(job.id);
  }
  return ids;
}

function errorStatus(cause: unknown) {
  if (typeof cause !== 'object' || cause == null || !('status' in cause)) {
    return undefined;
  }
  const status = (cause as { status: unknown }).status;
  return typeof status === 'number' ? status : undefined;
}

/**
 * 按点击时的 id 各请求一次，单条失败不打断其余任务。
 * 404 表示任务已经不在 dead；409 表示状态对不上，都不能当成重新入队。
 */
export async function retryDeadBatch(
  ids: readonly string[],
  retry: (id: string) => Promise<void>,
): Promise<RetryFeedback> {
  const feedback: RetryFeedback = {
    requeued: [],
    gone: [],
    conflict: [],
    failed: [],
    handledIDs: [],
  };
  for (const id of deadJobIDs(ids.map((id) => ({ id })))) {
    try {
      await retry(id);
      feedback.requeued.push(id);
      feedback.handledIDs.push(id);
    } catch (cause) {
      const status = errorStatus(cause);
      if (status === 404) {
        feedback.gone.push(id);
        feedback.handledIDs.push(id);
      } else if (status === 409) {
        feedback.conflict.push(id);
      } else {
        feedback.failed.push(id);
      }
    }
  }
  return feedback;
}

/** 成功只表示重新入队。剩余数量用刷新后的 dead，不用本次请求数代替。 */
export function confirmedRetryNote(feedback: RetryFeedback, dead: number) {
  const parts: string[] = [];
  if (feedback.requeued.length > 0) {
    parts.push(`已重新排队 ${feedback.requeued.length} 个`);
  }
  if (feedback.conflict.length > 0) {
    parts.push(`${feedback.conflict.length} 个状态已变化`);
  }
  if (feedback.failed.length > 0) {
    parts.push(`${feedback.failed.length} 个未能重试`);
  }
  if (dead > 0) {
    const remain = `还有 ${dead} 个失败任务，可继续重试`;
    return parts.length > 0 ? `${parts.join('，')}，${remain}` : remain;
  }
  if (feedback.conflict.length > 0 || feedback.failed.length > 0) {
    return `${parts.join('，')}。统计显示没有剩余失败任务`;
  }
  if (feedback.requeued.length > 0) return '已重新排队';
  return '没有剩余的失败任务';
}

/** 统计没刷新成功时，只报告这一轮请求结果，不推断剩余是 0。 */
export function unconfirmedRetryNote(feedback: RetryFeedback) {
  const parts: string[] = [];
  if (feedback.requeued.length > 0) {
    parts.push(`已重新排队 ${feedback.requeued.length} 个`);
  }
  if (feedback.conflict.length > 0) {
    parts.push(`${feedback.conflict.length} 个状态已变化`);
  }
  if (feedback.failed.length > 0) {
    parts.push(`${feedback.failed.length} 个未能重试`);
  }
  if (
    feedback.gone.length > 0 &&
    feedback.requeued.length === 0 &&
    feedback.conflict.length === 0 &&
    feedback.failed.length === 0
  ) {
    parts.push(`${feedback.gone.length} 个已不在失败列表`);
  }
  const head = parts.length > 0 ? `${parts.join('，')}。` : '';
  return `${head}统计刷新失败，不能确认剩余失败任务`;
}

export function composeRetryNote(
  feedback: RetryFeedback,
  dead: number | null,
  listFailed: boolean,
) {
  const base =
    dead == null
      ? unconfirmedRetryNote(feedback)
      : confirmedRetryNote(feedback, dead);
  return listFailed ? `${base}。记忆列表刷新失败` : base;
}

/**
 * 一次点击只重试调用时拿到的 id，然后刷新统计和列表。
 * 不根据刷新结果自动请求下一批，避免失败任务一直占住前 20 条时循环请求。
 */
export async function runDeadRetry<
  TStats extends { dead: number; deadJobs: readonly { id: string }[] },
  TList,
>(input: {
  stats: { dead: number; deadJobs: readonly { id: string }[] } | null;
  retry: (id: string) => Promise<void>;
  refreshStats: () => Promise<TStats>;
  refreshList: () => Promise<TList>;
}): Promise<DeadRetryResult<TStats, TList> | { ignored: true }> {
  const ids = input.stats ? deadJobIDs(input.stats.deadJobs) : [];
  if (ids.length === 0) return { ignored: true };
  const feedback = await retryDeadBatch(ids, input.retry);
  const [statsResult, listResult] = await Promise.allSettled([
    input.refreshStats(),
    input.refreshList(),
  ]);
  const stats = statsResult.status === 'fulfilled' ? statsResult.value : null;
  const list = listResult.status === 'fulfilled' ? listResult.value : null;
  const listFailed = listResult.status === 'rejected';
  return {
    ignored: false,
    feedback,
    stats,
    list,
    note: composeRetryNote(feedback, stats ? stats.dead : null, listFailed),
    alert:
      feedback.failed.length > 0 ||
      feedback.conflict.length > 0 ||
      stats == null ||
      listFailed,
    needsReload: stats == null || listFailed,
  };
}

/**
 * 手动新增的请求体。不生成、不附带 subject_key，连续两次不同事实各自只带内容。
 */
export function manualMemoryBody(input: {
  content: string;
  namespace: string;
  importance: number;
  kind?: string;
  pinned?: boolean;
  scope?: 'user' | 'agent';
  agent_id?: string;
}) {
  const body: {
    content: string;
    namespace: string;
    importance: number;
    kind?: string;
    pinned?: boolean;
    scope?: 'user' | 'agent';
    agent_id?: string;
  } = {
    content: input.content,
    namespace: input.namespace,
    importance: input.importance,
  };
  if (input.kind != null) body.kind = input.kind;
  if (input.pinned != null) body.pinned = input.pinned;
  if (input.scope != null) body.scope = input.scope;
  if (input.agent_id != null && input.agent_id !== '') {
    body.agent_id = input.agent_id;
  }
  return body;
}
