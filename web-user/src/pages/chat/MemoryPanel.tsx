import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { type AgentVersion, ApiError, errorMessage } from '@/services/api';
import {
  containsCredential,
  countMemories,
  createMemory,
  deleteMemory,
  getMemoryMarkdown,
  getMemoryStats,
  listMemories,
  listMemoryEvents,
  type MemoryCreate,
  type MemoryEdit,
  type MemoryItem,
  type MemoryPage,
  type MemoryPolicyResult,
  type MemoryStats,
  type MemoryView,
  readMemoryPolicy,
  restoreMemory,
  retryMemoryJob,
  saveMemoryPolicy,
  updateMemory,
} from '@/services/memory';
import {
  createRetryGate,
  deadJobIDs,
  runDeadRetry,
  withRetryGate,
} from '@/services/memoryRetry';
import { panelTime } from './AgentActivity';

/** 回复结束后抽取是异步的，稍等再读一次列表。 */
const REFRESH_AFTER_TURN_MS = 3000;
/** 侧栏一页大约一屏，剩下的靠底部滚动继续加载。 */
const PAGE_SIZE = 8;
const namespacePattern = /^[a-z][a-z0-9_.-]{0,63}$/;

/**
 * 预设分类的中文名。分组标题和下拉共用。
 * 未收录的 key 原样显示，避免把未知分类猜成别的意思。
 */
const namespaceLabels: Record<string, string> = {
  profile: '个人资料',
  preferences: '偏好',
  habits: '习惯',
  plans: '计划',
  notes: '笔记',
  style: '风格',
};

const presetNamespaces = [
  'profile',
  'preferences',
  'habits',
  'plans',
  'notes',
  'style',
] as const;

/** 下拉里的「自定义…」。这个值不是合法分类 key。 */
const customNamespaceValue = '~';

function namespaceLabel(key: string) {
  return namespaceLabels[key] ?? key;
}

/** 预设始终可选；当前列表里多出来的分类按 key 接在后面。 */
function namespaceChoices(existing: readonly string[]) {
  const extras = [...new Set(existing)]
    .filter((key) => key !== '' && !(key in namespaceLabels))
    .sort((a, b) => a.localeCompare(b));
  return [...presetNamespaces, ...extras];
}

const kindLabels: Record<string, string> = {
  fact: '事实',
  preference: '偏好',
  episode: '经历',
  procedure: '习惯/流程',
};

const kindTones: Record<string, string> = {
  fact: 'bg-zinc-100 text-zinc-600',
  preference: 'bg-violet-50 text-violet-700',
  episode: 'bg-sky-50 text-sky-700',
  procedure: 'bg-amber-50 text-amber-800',
};

const opLabels: Record<string, string> = {
  ADD: '新增',
  UPDATE: '更新',
  SUPERSEDE: '取代',
  DELETE: '删除',
  ARCHIVE: '归档',
  RESTORE: '恢复',
  MERGE: '合并',
};

const statusViews = [
  { id: 'active', label: '生效' },
  { id: 'archived', label: '已归档' },
  { id: 'deleted', label: '已删除' },
] as const;

type StatusView = (typeof statusViews)[number]['id'];

/** 分别保留工具写入和回复结束，避免同一批 SSE 事件覆盖前一条刷新提示。 */
export type MemoryCue = {
  agentId: string;
  seq: number;
  reason?: 'memory_updated' | 'turn_completed';
  memoryUpdateSeq?: number;
  turnCompleted?: { agentId: string; seq: number };
};

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (item: number) => String(item).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function sourceLabel(source: string) {
  if (source === 'auto') return '自动';
  if (source === 'user_edited') return '手动编辑';
  return source;
}

function kindLabel(kind: string) {
  return kindLabels[kind] ?? kind;
}

function opLabel(op: string) {
  return opLabels[op] ?? op;
}

function retentionPercent(value: number) {
  if (!Number.isFinite(value)) return 0;
  return Math.round(Math.min(1, Math.max(0, value)) * 100);
}

function validateDraft(draft: MemoryEdit) {
  if (draft.content.trim() === '') return '请填写记忆内容';
  if ([...draft.content].length > 2000) return '记忆内容不能超过 2000 字';
  if (containsCredential(draft.content))
    return '这段内容包含密码或证件号，不能保存';
  if (!namespacePattern.test(draft.namespace)) return '分类格式不正确';
  if (
    !Number.isInteger(draft.importance) ||
    draft.importance < 1 ||
    draft.importance > 10
  ) {
    return '重要度需要是 1 到 10 的整数';
  }
  return '';
}

function validateCreate(draft: MemoryCreate) {
  const message = validateDraft({
    content: draft.content,
    namespace: draft.namespace,
    importance: draft.importance,
    pinned: draft.pinned ?? false,
  });
  if (message) return message;
  if (
    draft.kind != null &&
    draft.kind !== 'fact' &&
    draft.kind !== 'preference' &&
    draft.kind !== 'episode' &&
    draft.kind !== 'procedure'
  ) {
    return '类型不正确';
  }
  if (draft.scope === 'agent' && !draft.agent_id) return '请选择这个 Agent';
  if (
    draft.scope != null &&
    draft.scope !== 'user' &&
    draft.scope !== 'agent'
  ) {
    return '保存位置不正确';
  }
  return '';
}

/** 同一 namespace 内置顶在前；分组标题按 namespace 排序。 */
function groupByNamespace(items: MemoryItem[]) {
  const sorted = [...items].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    if (a.importance !== b.importance) return b.importance - a.importance;
    return b.updated_at.localeCompare(a.updated_at);
  });
  const groups = new Map<string, MemoryItem[]>();
  for (const item of sorted) {
    const list = groups.get(item.namespace);
    if (list) list.push(item);
    else groups.set(item.namespace, [item]);
  }
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
}

function belongsToAgent(item: MemoryItem, agentId: string) {
  return (
    item.scope === 'user' ||
    (item.scope === 'agent' && item.agent_id === agentId)
  );
}

/** 设置与记忆共用的二态开关。 */
export function Switch({
  checked,
  disabled,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={(event) => {
        event.stopPropagation();
        onChange(!checked);
      }}
      className={`relative h-5 w-9 shrink-0 rounded-full transition-colors disabled:opacity-40 ${checked ? 'bg-zinc-900' : 'bg-zinc-300'}`}
    >
      <span
        className={`absolute top-0.5 left-0.5 size-4 rounded-full bg-white transition-transform ${checked ? 'translate-x-4' : ''}`}
      />
    </button>
  );
}

function Alert({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div
      className="mb-3 flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3 py-2 text-sm text-rose-800"
      role="alert"
    >
      {message}
      {onRetry && (
        <button
          type="button"
          className="shrink-0 text-xs underline"
          onClick={onRetry}
        >
          重新读取
        </button>
      )}
    </div>
  );
}

function Toast({ message }: { message: string }) {
  if (!message) return null;
  return (
    <p
      role="status"
      className="absolute top-14 left-1/2 z-10 -translate-x-1/2 rounded-full bg-zinc-900 px-3 py-1.5 text-xs whitespace-nowrap text-white shadow-sm"
    >
      {message}
    </p>
  );
}

type MemoryController = ReturnType<typeof useAgentMemory>;

/** 两层记忆一次分页读取，工具写入立即刷新，回复结束后再读取异步抽取结果。 */
export function useAgentMemory({
  agentId,
  version,
  editable,
  cue,
  onPolicySaved,
  onConflict,
}: {
  agentId: string;
  version?: AgentVersion;
  editable: boolean;
  cue: MemoryCue;
  onPolicySaved: (result: MemoryPolicyResult) => void;
  onConflict: () => void;
}) {
  const [revision, setRevision] = useState(0);
  const [status, setStatus] = useState<StatusView>('active');
  const [items, setItems] = useState<MemoryItem[]>([]);
  const [remembered, setRemembered] = useState<MemoryItem[]>([]);
  const [nextCursor, setNextCursor] = useState('');
  const [loaded, setLoaded] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [activeCount, setActiveCount] = useState<number | null>(null);
  const [stats, setStats] = useState<MemoryStats | null>(null);
  const [statsStale, setStatsStale] = useState(false);
  const [retrying, setRetrying] = useState(false);
  const [retryNote, setRetryNote] = useState('');
  const [retryAlert, setRetryAlert] = useState(false);
  const [retryNeedsReload, setRetryNeedsReload] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [policySaving, setPolicySaving] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [preview, setPreview] = useState(false);
  const [markdown, setMarkdown] = useState<MemoryView>();
  const [markdownLoading, setMarkdownLoading] = useState(false);
  const generation = useRef(0);
  const statsEpoch = useRef(0);
  const completedTimer = useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined,
  );
  const memoryUpdateSeq =
    cue.memoryUpdateSeq ?? (cue.reason === 'memory_updated' ? cue.seq : 0);
  const seenMemoryUpdate = useRef(memoryUpdateSeq);
  const loadingMoreRef = useRef(false);
  const retryGate = useRef(createRetryGate());
  const statsRef = useRef(stats);
  const agentRef = useRef(agentId);
  statsRef.current = stats;
  const onPolicySavedRef = useRef(onPolicySaved);
  const onConflictRef = useRef(onConflict);
  const versionRef = useRef(version);
  onPolicySavedRef.current = onPolicySaved;
  onConflictRef.current = onConflict;
  versionRef.current = version;
  const policy = readMemoryPolicy(version?.configuration.memory_policy);
  const visibleItems = useMemo(() => {
    const seen = new Set(items.map((item) => item.id));
    const head = remembered.filter(
      (item) =>
        item.status === status &&
        !seen.has(item.id) &&
        belongsToAgent(item, agentId),
    );
    return [...head, ...items.filter((item) => item.status === status)];
  }, [agentId, items, remembered, status]);

  const requestRefresh = useCallback(() => {
    generation.current += 1;
    statsEpoch.current += 1;
    setRevision((value) => value + 1);
  }, []);

  useEffect(() => {
    agentRef.current = agentId;
    setRemembered([]);
    setStatus('active');
    setActiveCount(null);
    setStats(null);
    setStatsStale(false);
    setRetrying(false);
    setRetryNote('');
    setRetryAlert(false);
    setRetryNeedsReload(false);
    return () => {
      agentRef.current = '';
      clearTimeout(completedTimer.current);
    };
  }, [agentId]);

  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(''), 2600);
    return () => clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    if (seenMemoryUpdate.current === memoryUpdateSeq) return;
    seenMemoryUpdate.current = memoryUpdateSeq;
    // 事件没有作用域，使用当前 Agent 重新读取共享层和专属层。旧的本地条目可能已被删除或取代。
    setRemembered([]);
    requestRefresh();
  }, [memoryUpdateSeq, requestRefresh]);

  const completedAgent = cue.turnCompleted?.agentId ?? cue.agentId;
  const completedSeq =
    cue.turnCompleted?.seq ?? (cue.reason === 'memory_updated' ? 0 : cue.seq);
  useEffect(() => {
    if (completedSeq === 0 || completedAgent !== agentId) return;
    // 其他 Agent 的完成提示不影响当前 Agent 已经安排的读取。
    clearTimeout(completedTimer.current);
    completedTimer.current = setTimeout(requestRefresh, REFRESH_AFTER_TURN_MS);
  }, [agentId, completedAgent, completedSeq, requestRefresh]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 在对话结束和编辑后触发重新读取。重试自己刷新列表。
  useEffect(() => {
    const current = ++generation.current;
    const controller = new AbortController();
    setError('');
    listMemories(
      {
        scope: 'all_visible',
        agent_id: agentId,
        status,
        limit: PAGE_SIZE,
        cursor: '',
      },
      controller.signal,
    )
      .then((page) => {
        if (controller.signal.aborted || generation.current !== current) return;
        setItems(page.items);
        setNextCursor(page.nextCursor);
        setLoaded(true);
      })
      .catch((cause) => {
        if (controller.signal.aborted || generation.current !== current) return;
        setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agentId, revision, status]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 变化后重新统计生效条数，卡片不随筛选改变。
  useEffect(() => {
    const epoch = statsEpoch.current;
    const controller = new AbortController();
    countMemories(
      { scope: 'all_visible', agent_id: agentId, status: 'active' },
      controller.signal,
    )
      .then((total) => {
        if (controller.signal.aborted || statsEpoch.current !== epoch) return;
        setActiveCount(total);
      })
      .catch((cause) => {
        if (controller.signal.aborted || statsEpoch.current !== epoch) return;
        setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agentId, revision]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: 打开详情和刷新后读取统计。
  useEffect(() => {
    if (!detailOpen) return;
    const epoch = statsEpoch.current;
    const controller = new AbortController();
    getMemoryStats(controller.signal)
      .then((value) => {
        if (controller.signal.aborted || statsEpoch.current !== epoch) return;
        setStats(value);
        setStatsStale(false);
      })
      .catch((cause) => {
        if (controller.signal.aborted || statsEpoch.current !== epoch) return;
        setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agentId, detailOpen, revision]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: 开关和 revision 变化后重新读取 memory.md。
  useEffect(() => {
    if (!preview) return;
    const controller = new AbortController();
    setMarkdown(undefined);
    setMarkdownLoading(true);
    getMemoryMarkdown(agentId, controller.signal)
      .then((view) => {
        if (!controller.signal.aborted) setMarkdown(view);
      })
      .catch((cause) => {
        if (controller.signal.aborted) return;
        setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setMarkdownLoading(false);
      });
    return () => controller.abort();
  }, [agentId, policy.enabled, preview, revision]);

  const refresh = () => {
    setRetryNote('');
    setRetryAlert(false);
    setRetryNeedsReload(false);
    requestRefresh();
  };
  const reportConflict = () => {
    setNotice('该记忆已被更新，已刷新');
    requestRefresh();
  };
  const notify = (message: string) => setNotice(message);
  const changeStatus = (next: StatusView) => {
    if (next === status) return;
    generation.current += 1;
    setStatus(next);
    setLoaded(false);
    setItems([]);
    setNextCursor('');
  };
  const remember = (item: MemoryItem) => {
    setRemembered((current) => [
      item,
      ...current.filter((entry) => entry.id !== item.id),
    ]);
    requestRefresh();
  };
  const forget = (id: string) => {
    setRemembered((current) => current.filter((item) => item.id !== id));
  };
  const loadMore = useCallback(async () => {
    if (!nextCursor || loadingMoreRef.current) return;
    const current = generation.current;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    setError('');
    try {
      const page = await listMemories({
        scope: 'all_visible',
        agent_id: agentId,
        status,
        limit: PAGE_SIZE,
        cursor: nextCursor,
      });
      if (generation.current !== current) return;
      setItems((currentItems) => {
        const seen = new Set(currentItems.map((item) => item.id));
        return [
          ...currentItems,
          ...page.items.filter((item) => !seen.has(item.id)),
        ];
      });
      setNextCursor(page.nextCursor);
    } catch (cause) {
      if (generation.current === current) setError(errorMessage(cause));
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }, [agentId, nextCursor, status]);
  const toggle = async () => {
    const current = versionRef.current;
    if (!current || !editable || policySaving) return;
    const enabled = readMemoryPolicy(
      current.configuration.memory_policy,
    ).enabled;
    setPolicySaving(true);
    setError('');
    try {
      const result = await saveMemoryPolicy(agentId, {
        expected_version_id: current.id,
        enabled: !enabled,
      });
      onPolicySavedRef.current(result);
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        onConflictRef.current();
        reportConflict();
        return;
      }
      setError(errorMessage(cause));
    } finally {
      setPolicySaving(false);
    }
  };
  const saveAdvanced = async (input: {
    decay_enabled: boolean;
    recall_top_k: number;
  }) => {
    const current = versionRef.current;
    if (!current || !editable || policySaving) return;
    if (
      !Number.isInteger(input.recall_top_k) ||
      input.recall_top_k < 1 ||
      input.recall_top_k > 50
    ) {
      setError('召回条数需要是 1 到 50 的整数');
      return;
    }
    setPolicySaving(true);
    setError('');
    try {
      const enabled = readMemoryPolicy(
        current.configuration.memory_policy,
      ).enabled;
      const result = await saveMemoryPolicy(agentId, {
        expected_version_id: current.id,
        enabled,
        recall_top_k: input.recall_top_k,
        decay_enabled: input.decay_enabled,
      });
      onPolicySavedRef.current(result);
      setNotice('已保存');
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        onConflictRef.current();
        reportConflict();
        return;
      }
      setError(errorMessage(cause));
    } finally {
      setPolicySaving(false);
    }
  };
  const retryDead = () => {
    // 进入闸门后才改界面。重复点击在这里返回，不会把进行中的按钮重新放开。
    void withRetryGate(retryGate.current, async () => {
      const current = statsRef.current;
      if (
        !current ||
        (current.dead <= 0 && deadJobIDs(current.deadJobs).length === 0)
      ) {
        return;
      }
      // 丢掉点击前还在飞的统计和列表，避免过期响应把刚重试过的 id 写回来。
      const listGeneration = ++generation.current;
      const retryStatsEpoch = ++statsEpoch.current;
      setRetrying(true);
      setRetryNote('');
      setRetryAlert(false);
      setRetryNeedsReload(false);
      setError('');
      try {
        const outcome = await runDeadRetry<
          MemoryStats,
          { page: MemoryPage; total: number }
        >({
          stats: current,
          retry: retryMemoryJob,
          refreshStats: () => getMemoryStats(),
          refreshList: async () => {
            const page = await listMemories({
              scope: 'all_visible',
              agent_id: agentId,
              status,
              limit: PAGE_SIZE,
              cursor: '',
            });
            const total = await countMemories({
              scope: 'all_visible',
              agent_id: agentId,
              status: 'active',
            });
            return { page, total };
          },
        });
        if (outcome.ignored || agentRef.current !== agentId) return;
        if (statsEpoch.current !== retryStatsEpoch) {
          // 有更新的读取时不覆盖它。重试已经结束，再读取一次以纳入刚提交的任务状态。
          requestRefresh();
          return;
        }
        if (outcome.stats) {
          setStats(outcome.stats);
          setStatsStale(false);
        } else {
          const handled = new Set(outcome.feedback.handledIDs);
          setStats((prev) =>
            prev
              ? {
                  ...prev,
                  deadJobs: prev.deadJobs.filter((job) => !handled.has(job.id)),
                }
              : prev,
          );
          setStatsStale(true);
        }
        if (outcome.list) {
          if (generation.current === listGeneration) {
            setItems(outcome.list.page.items);
            setNextCursor(outcome.list.page.nextCursor);
            setLoaded(true);
          }
          setActiveCount(outcome.list.total);
        }
        setRetryNote(outcome.note);
        setRetryAlert(outcome.alert);
        setRetryNeedsReload(outcome.needsReload);
      } catch (cause) {
        if (
          agentRef.current === agentId &&
          statsEpoch.current === retryStatsEpoch
        )
          setError(errorMessage(cause));
      } finally {
        if (agentRef.current === agentId) setRetrying(false);
      }
    });
  };

  return {
    items: visibleItems,
    loaded,
    count: activeCount,
    hasMore: nextCursor !== '',
    loadingMore,
    status,
    changeStatus,
    stats,
    statsStale,
    retrying,
    retryNote,
    retryAlert,
    retryNeedsReload,
    error,
    notice,
    policy,
    policySaving,
    detailOpen,
    setDetailOpen,
    preview,
    setPreview,
    markdown,
    markdownLoading,
    editable,
    refresh,
    reportConflict,
    notify,
    remember,
    forget,
    loadMore,
    toggle,
    saveAdvanced,
    retryDead,
  };
}

/** 侧栏记忆页优先展示内容，完整管理仍可从此处进入。 */
export function MemoryRows({ memory }: { memory: MemoryController }) {
  const [expanded, setExpanded] = useState('');
  const [deleting, setDeleting] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const remove = async (id: string) => {
    setBusy(true);
    setError('');
    try {
      await deleteMemory(id);
      memory.forget(id);
      setDeleting('');
      memory.refresh();
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <div className="flex min-h-12 items-center justify-between gap-3">
        <span>
          {memory.count === null ? '记忆' : `记住了 ${memory.count} 件`}
        </span>
        <Switch
          checked={memory.policy.enabled}
          disabled={!memory.editable || memory.policySaving}
          label={memory.policy.enabled ? '关闭记忆' : '开启记忆'}
          onChange={() => void memory.toggle()}
        />
      </div>
      {memory.error && (
        <Alert message={memory.error} onRetry={memory.refresh} />
      )}
      {error && (
        <p role="alert" className="py-2 text-rose-700">
          {error}
        </p>
      )}
      {!memory.loaded && !memory.error && <Skeleton className="h-20" />}
      {memory.loaded && !memory.items.length && (
        <p className="py-3 text-zinc-500">暂无记忆</p>
      )}
      {memory.items.map((item) => {
        const open = expanded === item.id;
        const { group, time } = panelTime(item.created_at);
        return (
          <div key={item.id} className="py-2.5">
            <div className="flex items-start gap-2">
              <button
                type="button"
                aria-expanded={open}
                className="min-w-0 flex-1 text-left"
                onClick={() => setExpanded(open ? '' : item.id)}
              >
                <span
                  className={
                    open
                      ? 'block whitespace-pre-wrap break-words'
                      : 'block truncate'
                  }
                >
                  {item.content}
                </span>
                <time
                  dateTime={item.created_at}
                  className="mt-0.5 block text-xs text-zinc-500"
                >
                  {group === '更早' ? time : `${group} ${time}`}
                </time>
              </button>
              <button
                type="button"
                aria-label="删除记忆"
                disabled={busy}
                className="flex size-7 shrink-0 items-center justify-center rounded text-zinc-500 hover:bg-zinc-100"
                onClick={() => setDeleting(item.id)}
              >
                <Icon name="trash" size={15} />
              </button>
            </div>
            {deleting === item.id && (
              <div className="mt-2 flex items-center justify-between gap-2 text-xs">
                <span>删除这条记忆？</span>
                <div className="flex gap-3">
                  <button
                    type="button"
                    disabled={busy}
                    onClick={() => setDeleting('')}
                  >
                    取消
                  </button>
                  <button
                    type="button"
                    disabled={busy}
                    className="text-rose-700"
                    onClick={() => void remove(item.id)}
                  >
                    删除
                  </button>
                </div>
              </div>
            )}
          </div>
        );
      })}
      {memory.hasMore && (
        <button
          type="button"
          className="my-2 block text-xs text-zinc-500 hover:underline"
          disabled={memory.loadingMore}
          onClick={() => void memory.loadMore()}
        >
          {memory.loadingMore ? '加载中…' : '加载更多'}
        </button>
      )}
      <button
        type="button"
        className="mt-3 text-xs text-zinc-500 hover:underline"
        onClick={() => memory.setDetailOpen(true)}
      >
        管理记忆
      </button>
    </>
  );
}

function MemoryHistory({ id }: { id: string }) {
  const [events, setEvents] = useState<
    Awaited<ReturnType<typeof listMemoryEvents>> | undefined
  >();
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    listMemoryEvents(id, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setEvents(result);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [id]);
  if (error) return <Alert message={error} />;
  if (!events) return <Skeleton className="mt-3 h-10" />;
  if (events.length === 0) {
    return <p className="mt-3 text-xs text-zinc-400">没有历史</p>;
  }
  return (
    <ol className="mt-3 space-y-2 border-l border-zinc-200 pl-3">
      {events.map((event) => (
        <li key={event.id}>
          <p className="text-xs font-medium text-zinc-800">
            {opLabel(event.op)}
          </p>
          <p className="text-[11px] text-zinc-500">
            <time dateTime={event.created_at}>
              {formatTime(event.created_at)}
            </time>
          </p>
          {event.reason.trim() && (
            <p className="text-xs text-zinc-600">{event.reason}</p>
          )}
        </li>
      ))}
    </ol>
  );
}

function NamespaceField({
  value,
  existing,
  disabled,
  onChange,
}: {
  value: string;
  existing: readonly string[];
  disabled?: boolean;
  onChange: (namespace: string) => void;
}) {
  const options = namespaceChoices(existing);
  const [custom, setCustom] = useState(
    () => !namespaceChoices(existing).includes(value),
  );
  const showCustom = custom || !options.includes(value);
  return (
    <div className="text-xs text-zinc-500">
      分类
      <Select
        value={showCustom ? customNamespaceValue : value}
        disabled={disabled}
        onValueChange={(next) => {
          if (next === customNamespaceValue) {
            setCustom(true);
            if (options.includes(value)) onChange('');
            return;
          }
          setCustom(false);
          onChange(next);
        }}
      >
        <SelectTrigger className="mt-1" aria-label="分类">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((key) => (
            <SelectItem key={key} value={key}>
              {namespaceLabel(key)}
            </SelectItem>
          ))}
          <SelectItem value={customNamespaceValue}>自定义…</SelectItem>
        </SelectContent>
      </Select>
      {showCustom && (
        <input
          className="mt-1 h-8 w-full rounded-lg border border-zinc-200 bg-white px-2 text-sm text-zinc-900"
          aria-label="自定义分类"
          value={value}
          disabled={disabled}
          autoComplete="off"
          spellCheck={false}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </div>
  );
}

function MemoryItemView({
  item,
  namespaces,
  enabled,
  onChanged,
  onConflict,
  onRemember,
  onForget,
}: {
  item: MemoryItem;
  namespaces: readonly string[];
  enabled: boolean;
  onChanged: () => void;
  onConflict: () => void;
  onRemember: (item: MemoryItem) => void;
  onForget: (id: string) => void;
}) {
  const [mode, setMode] = useState<'view' | 'edit' | 'delete'>('view');
  const [history, setHistory] = useState(false);
  const [draft, setDraft] = useState<MemoryEdit>({
    content: item.content,
    namespace: item.namespace,
    importance: item.importance,
    pinned: item.pinned,
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');
  const inactive = item.status === 'archived' || item.status === 'deleted';
  const percent = retentionPercent(item.retention);
  const save = async () => {
    const message = validateDraft(draft);
    if (message) {
      setFormError(message);
      return;
    }
    setSaving(true);
    setFormError('');
    try {
      const next = await updateMemory(item.id, draft);
      setMode('view');
      onForget(item.id);
      onRemember(next);
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        setMode('view');
        onConflict();
        return;
      }
      if (
        cause instanceof ApiError &&
        cause.status === 422 &&
        containsCredential(draft.content)
      ) {
        setFormError('这段内容包含密码或证件号，不能保存');
        return;
      }
      setFormError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  const remove = async () => {
    setSaving(true);
    setFormError('');
    try {
      await deleteMemory(item.id);
      onForget(item.id);
      onChanged();
    } catch (cause) {
      setFormError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  const restore = async () => {
    setSaving(true);
    setFormError('');
    try {
      const next = await restoreMemory(item.id);
      onForget(item.id);
      onRemember(next);
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        setFormError('相同事实已存在');
        onChanged();
        return;
      }
      setFormError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  return (
    <article className="rounded-2xl border border-zinc-200/80 bg-white/80 p-3">
      {mode === 'edit' ? (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}
        >
          <label className="block text-xs text-zinc-500">
            内容
            <textarea
              className="mt-1 min-h-24 w-full resize-none rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900"
              aria-label="记忆内容"
              maxLength={2000}
              value={draft.content}
              disabled={saving}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  content: event.target.value,
                }))
              }
            />
          </label>
          <NamespaceField
            value={draft.namespace}
            existing={namespaces}
            disabled={saving}
            onChange={(namespace) =>
              setDraft((current) => ({ ...current, namespace }))
            }
          />
          <div className="text-xs text-zinc-500">
            重要度
            <Select
              value={String(draft.importance)}
              disabled={saving}
              onValueChange={(value) =>
                setDraft((current) => ({
                  ...current,
                  importance: Number(value),
                }))
              }
            >
              <SelectTrigger className="mt-1" aria-label="重要度">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 10 }, (_, index) =>
                  String(index + 1),
                ).map((value) => (
                  <SelectItem key={value} value={value}>
                    {value}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-center gap-2 text-xs text-zinc-600">
            <input
              type="checkbox"
              checked={draft.pinned}
              disabled={saving}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  pinned: event.target.checked,
                }))
              }
            />
            置顶
          </label>
          {formError && (
            <p className="text-xs text-rose-700" role="alert">
              {formError}
            </p>
          )}
          <div className="flex gap-2">
            <Button size="sm" type="submit" disabled={saving}>
              {saving ? '保存中…' : '保存'}
            </Button>
            <Button
              size="sm"
              variant="quiet"
              disabled={saving}
              onClick={() => {
                setDraft({
                  content: item.content,
                  namespace: item.namespace,
                  importance: item.importance,
                  pinned: item.pinned,
                });
                setFormError('');
                setMode('view');
              }}
            >
              取消
            </Button>
          </div>
        </form>
      ) : (
        <>
          <div className="flex items-start gap-1.5">
            {item.sensitive && (
              <>
                <Icon
                  name="lock"
                  size={14}
                  className="mt-1 shrink-0 text-zinc-500"
                />
                <span className="sr-only">敏感</span>
              </>
            )}
            {item.pinned && (
              <>
                <Icon
                  name="pin"
                  size={14}
                  className="mt-1 shrink-0 text-zinc-500"
                />
                <span className="sr-only">已置顶</span>
              </>
            )}
            <p className="text-sm leading-6 whitespace-pre-wrap text-zinc-800">
              {item.content}
            </p>
          </div>
          <p className="mt-2 flex flex-wrap items-center gap-1.5 text-[11px] text-zinc-500">
            <span
              className={`rounded-full px-1.5 py-0.5 ${kindTones[item.kind] ?? 'bg-zinc-100 text-zinc-600'}`}
            >
              {kindLabel(item.kind)}
            </span>
            <span>重要度 {item.importance}</span>
            <span className="rounded-full bg-zinc-100 px-1.5 py-0.5 text-zinc-600">
              {sourceLabel(item.source)}
            </span>
            <time dateTime={item.updated_at}>
              {formatTime(item.updated_at)}
            </time>
          </p>
          <div className="mt-2 flex items-center gap-2">
            <span
              className="shrink-0 text-[11px] text-zinc-400"
              id={`memory-strength-${item.id}`}
            >
              记忆强度
            </span>
            <progress
              className="h-1 min-w-0 flex-1 appearance-none overflow-hidden rounded-full bg-zinc-200 accent-zinc-800 [&::-moz-progress-bar]:bg-zinc-800 [&::-webkit-progress-bar]:rounded-full [&::-webkit-progress-bar]:bg-zinc-200 [&::-webkit-progress-value]:rounded-full [&::-webkit-progress-value]:bg-zinc-800"
              max={100}
              value={percent}
              aria-labelledby={`memory-strength-${item.id}`}
            />
          </div>
          {formError && (
            <p className="mt-2 text-xs text-rose-700" role="alert">
              {formError}
            </p>
          )}
          {mode === 'delete' ? (
            <div className="mt-2 flex items-center justify-between gap-2 text-xs">
              <span className="text-zinc-700">删除这条记忆？</span>
              <span className="flex gap-3">
                <button
                  type="button"
                  className="text-rose-700 underline underline-offset-4"
                  disabled={saving}
                  onClick={() => void remove()}
                >
                  删除
                </button>
                <button
                  type="button"
                  className="text-zinc-500 underline underline-offset-4"
                  disabled={saving}
                  onClick={() => setMode('view')}
                >
                  取消
                </button>
              </span>
            </div>
          ) : (
            <div className="mt-2 flex gap-3 text-xs">
              {inactive ? (
                <button
                  type="button"
                  className="text-zinc-800 underline underline-offset-4 disabled:opacity-40"
                  disabled={saving}
                  onClick={() => void restore()}
                >
                  {saving ? '恢复中…' : '恢复'}
                </button>
              ) : (
                <>
                  <button
                    type="button"
                    className="text-zinc-800 underline underline-offset-4 disabled:opacity-40"
                    disabled={!enabled}
                    onClick={() => {
                      setFormError('');
                      setMode('edit');
                    }}
                  >
                    编辑
                  </button>
                  <button
                    type="button"
                    className="text-zinc-800 underline underline-offset-4 disabled:opacity-40"
                    disabled={!enabled}
                    onClick={() => {
                      setFormError('');
                      setMode('delete');
                    }}
                  >
                    删除
                  </button>
                </>
              )}
              <button
                type="button"
                className="text-zinc-800 underline underline-offset-4"
                aria-expanded={history}
                onClick={() => setHistory((value) => !value)}
              >
                历史
              </button>
            </div>
          )}
          {history && <MemoryHistory id={item.id} />}
        </>
      )}
    </article>
  );
}

function MemoryGroup({
  title,
  items,
  namespaces,
  hasMore,
  enabled,
  onChanged,
  onConflict,
  onRemember,
  onForget,
}: {
  title: string;
  items: MemoryItem[];
  namespaces: readonly string[];
  hasMore: boolean;
  enabled: boolean;
  onChanged: () => void;
  onConflict: () => void;
  onRemember: (item: MemoryItem) => void;
  onForget: (id: string) => void;
}) {
  if (items.length === 0 && hasMore) return null;
  const groups = groupByNamespace(items);
  return (
    <section className="mt-5 first:mt-0">
      <h3 className="text-sm font-medium">{title}</h3>
      {items.length === 0 ? (
        <p className="mt-2 text-xs text-zinc-400">暂无</p>
      ) : (
        groups.map(([namespace, grouped]) => (
          <div key={namespace} className="mt-3">
            <h4 className="mb-2 text-[11px] font-medium text-zinc-400">
              {namespaceLabel(namespace)}
            </h4>
            <div className="space-y-2">
              {grouped.map((item) => (
                <MemoryItemView
                  key={item.id}
                  item={item}
                  namespaces={namespaces}
                  enabled={enabled}
                  onChanged={onChanged}
                  onConflict={onConflict}
                  onRemember={onRemember}
                  onForget={onForget}
                />
              ))}
            </div>
          </div>
        ))
      )}
    </section>
  );
}

function MemoryCreateForm({
  agentId,
  namespaces,
  onCancel,
  onCreated,
  onConflict,
}: {
  agentId: string;
  namespaces: readonly string[];
  onCancel: () => void;
  onCreated: (item: MemoryItem) => void;
  onConflict: () => void;
}) {
  const [draft, setDraft] = useState<MemoryCreate>({
    content: '',
    namespace: 'profile',
    importance: 5,
    kind: 'fact',
    pinned: false,
    scope: 'user',
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');
  const save = async () => {
    const input: MemoryCreate = {
      content: draft.content,
      namespace: draft.namespace,
      importance: draft.importance,
      kind: draft.kind,
      pinned: draft.pinned,
      scope: draft.scope,
      agent_id: draft.scope === 'agent' ? agentId : undefined,
    };
    const message = validateCreate(input);
    if (message) {
      setFormError(message);
      return;
    }
    setSaving(true);
    setFormError('');
    try {
      onCreated(await createMemory(input));
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        setFormError('相同事实已存在');
        onConflict();
        return;
      }
      if (
        cause instanceof ApiError &&
        cause.status === 422 &&
        containsCredential(draft.content)
      ) {
        setFormError('这段内容包含密码或证件号，不能保存');
        return;
      }
      setFormError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  return (
    <form
      className="mb-4 space-y-2 rounded-2xl border border-zinc-200 bg-zinc-50 p-3"
      onSubmit={(event) => {
        event.preventDefault();
        void save();
      }}
    >
      <label className="block text-xs text-zinc-500">
        内容
        <textarea
          className="mt-1 min-h-24 w-full resize-none rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900"
          aria-label="记忆内容"
          maxLength={2000}
          value={draft.content}
          disabled={saving}
          onChange={(event) =>
            setDraft((current) => ({ ...current, content: event.target.value }))
          }
        />
      </label>
      <NamespaceField
        value={draft.namespace}
        existing={namespaces}
        disabled={saving}
        onChange={(namespace) =>
          setDraft((current) => ({ ...current, namespace }))
        }
      />
      <div className="grid grid-cols-2 gap-2">
        <div className="text-xs text-zinc-500">
          类型
          <Select
            value={draft.kind}
            disabled={saving}
            onValueChange={(value) =>
              setDraft((current) => ({
                ...current,
                kind: value as MemoryCreate['kind'],
              }))
            }
          >
            <SelectTrigger className="mt-1" aria-label="类型">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(kindLabels).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="text-xs text-zinc-500">
          重要度
          <Select
            value={String(draft.importance)}
            disabled={saving}
            onValueChange={(value) =>
              setDraft((current) => ({
                ...current,
                importance: Number(value),
              }))
            }
          >
            <SelectTrigger className="mt-1" aria-label="重要度">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Array.from({ length: 10 }, (_, index) => String(index + 1)).map(
                (value) => (
                  <SelectItem key={value} value={value}>
                    {value}
                  </SelectItem>
                ),
              )}
            </SelectContent>
          </Select>
        </div>
      </div>
      <fieldset
        className="m-0 flex w-fit rounded-full border-0 bg-zinc-100 p-0.5 text-xs"
        aria-label="保存到"
      >
        <button
          type="button"
          className={`rounded-full px-2.5 py-1 ${draft.scope === 'user' ? 'bg-white text-zinc-900 shadow-sm' : 'text-zinc-500'}`}
          aria-pressed={draft.scope === 'user'}
          disabled={saving}
          onClick={() => setDraft((current) => ({ ...current, scope: 'user' }))}
        >
          关于你
        </button>
        <button
          type="button"
          className={`rounded-full px-2.5 py-1 ${draft.scope === 'agent' ? 'bg-white text-zinc-900 shadow-sm' : 'text-zinc-500'}`}
          aria-pressed={draft.scope === 'agent'}
          disabled={saving}
          onClick={() =>
            setDraft((current) => ({ ...current, scope: 'agent' }))
          }
        >
          这个 Agent
        </button>
      </fieldset>
      <label className="flex items-center gap-2 text-xs text-zinc-600">
        <input
          type="checkbox"
          checked={draft.pinned ?? false}
          disabled={saving}
          onChange={(event) =>
            setDraft((current) => ({
              ...current,
              pinned: event.target.checked,
            }))
          }
        />
        置顶
      </label>
      {formError && (
        <p className="text-xs text-rose-700" role="alert">
          {formError}
        </p>
      )}
      <div className="flex gap-2">
        <Button size="sm" type="submit" disabled={saving}>
          {saving ? '保存中…' : '保存'}
        </Button>
        <Button size="sm" variant="quiet" disabled={saving} onClick={onCancel}>
          取消
        </Button>
      </div>
    </form>
  );
}

function MemoryAdvanced({ memory }: { memory: MemoryController }) {
  const [decay, setDecay] = useState(memory.policy.decay_enabled);
  const [recall, setRecall] = useState(String(memory.policy.recall_top_k));
  useEffect(() => {
    setDecay(memory.policy.decay_enabled);
    setRecall(String(memory.policy.recall_top_k));
  }, [memory.policy.decay_enabled, memory.policy.recall_top_k]);
  return (
    <details className="mb-4 rounded-2xl border border-zinc-200 bg-white px-3 py-2">
      <summary className="cursor-pointer text-sm text-zinc-700">
        高级设置
      </summary>
      <form
        className="mt-3 space-y-3"
        onSubmit={(event) => {
          event.preventDefault();
          void memory.saveAdvanced({
            decay_enabled: decay,
            recall_top_k: Number(recall),
          });
        }}
      >
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-zinc-600">遗忘开关</span>
          <Switch
            checked={decay}
            disabled={!memory.editable || memory.policySaving}
            label="遗忘开关"
            onChange={setDecay}
          />
        </div>
        <label className="block text-xs text-zinc-600">
          召回条数
          <input
            className="mt-1 h-8 w-full rounded-lg border border-zinc-200 bg-white px-2 text-sm text-zinc-900"
            aria-label="召回条数"
            inputMode="numeric"
            min={1}
            max={50}
            value={recall}
            disabled={!memory.editable || memory.policySaving}
            onChange={(event) => setRecall(event.target.value)}
          />
        </label>
        {memory.editable && (
          <Button size="sm" type="submit" disabled={memory.policySaving}>
            {memory.policySaving ? '保存中…' : '保存'}
          </Button>
        )}
      </form>
    </details>
  );
}

function emptyCopy(status: StatusView) {
  if (status === 'archived') return '没有已归档的记忆';
  if (status === 'deleted') return '没有已删除的记忆';
  return '还没有记忆，多聊几句后会自动整理。';
}

/** 记忆详情占满整个配置栏，编辑、新增和恢复都留在这一层。 */
export function MemoryDetail({
  memory,
  agentId,
  onBack,
}: {
  memory: MemoryController;
  agentId: string;
  onBack: () => void;
}) {
  const [adding, setAdding] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const sentinelRef = useRef<HTMLButtonElement>(null);
  const enabled = memory.policy.enabled;
  const namespaces = useMemo(
    () => memory.items.map((item) => item.namespace),
    [memory.items],
  );
  const userItems = memory.items.filter((item) => item.scope === 'user');
  const agentItems = memory.items.filter((item) => item.scope === 'agent');
  const showEmpty =
    memory.loaded && memory.items.length === 0 && !memory.hasMore;
  useEffect(() => {
    const root = scrollRef.current;
    const target = sentinelRef.current;
    if (!root || !target || !memory.hasMore || memory.preview) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          void memory.loadMore();
        }
      },
      { root, rootMargin: '120px' },
    );
    observer.observe(target);
    return () => observer.disconnect();
  }, [memory.hasMore, memory.loadMore, memory.preview]);
  const created = (item: MemoryItem) => {
    setAdding(false);
    if (memory.status !== 'active') memory.changeStatus('active');
    memory.remember(item);
    memory.notify('已添加');
  };
  const kept = (item: MemoryItem) => {
    if (memory.status !== 'active') {
      memory.changeStatus('active');
      memory.notify('已恢复');
    }
    memory.remember(item);
  };
  const conflicted = () => {
    memory.reportConflict();
  };
  return (
    <section
      className="relative flex h-full min-h-0 flex-col bg-white"
      aria-label="Agent 配置"
    >
      <Toast message={memory.notice} />
      <header className="shrink-0 space-y-2 px-4 py-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="inline-flex items-center gap-1 text-sm text-zinc-600"
            onClick={onBack}
          >
            <Icon name="arrow-left" size={16} />
            返回
          </button>
          <h2 className="text-sm font-medium">记忆</h2>
          <button
            type="button"
            className="ml-auto text-sm text-zinc-800"
            onClick={() => setAdding((value) => !value)}
          >
            + 新增记忆
          </button>
        </div>
        <p className="text-[11px] leading-5 text-zinc-500">
          {memory.stats ? (
            <>
              生效 {memory.stats.status.active ?? 0} · 归档{' '}
              {memory.stats.status.archived ?? 0}
              {memory.stats.dead > 0 && (
                <>
                  {' · '}
                  {memory.stats.dead} 个整理任务失败
                  {memory.statsStale ? '（统计未更新）' : ''}
                  <button
                    type="button"
                    className="ml-1 underline underline-offset-2 disabled:opacity-40"
                    disabled={memory.retrying}
                    aria-busy={memory.retrying}
                    onClick={memory.retryDead}
                  >
                    {memory.retrying ? '重试中…' : '重试'}
                  </button>
                </>
              )}
            </>
          ) : (
            '…'
          )}
        </p>
        {memory.retryNote && (
          <p
            className={`text-[11px] leading-5 ${memory.retryAlert ? 'text-rose-700' : 'text-zinc-500'}`}
            role={memory.retryAlert ? 'alert' : 'status'}
          >
            {memory.retryNote}
            {memory.retryNeedsReload && (
              <button
                type="button"
                className="ml-1 underline underline-offset-2"
                onClick={memory.refresh}
              >
                重新读取
              </button>
            )}
          </p>
        )}
        <div className="flex items-center justify-end gap-2 text-xs text-zinc-600">
          <span>memory.md 预览</span>
          <Switch
            checked={memory.preview}
            label="memory.md 预览"
            onChange={memory.setPreview}
          />
        </div>
      </header>
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto px-4 pb-6">
        {memory.error && (
          <Alert message={memory.error} onRetry={memory.refresh} />
        )}
        {!enabled && (
          <p className="mb-4 rounded-xl bg-zinc-100 px-3 py-2 text-xs leading-5 text-zinc-600">
            记忆已关闭，这些内容不会参与后续对话。
          </p>
        )}
        {adding && (
          <MemoryCreateForm
            agentId={agentId}
            namespaces={namespaces}
            onCancel={() => setAdding(false)}
            onCreated={created}
            onConflict={() => memory.refresh()}
          />
        )}
        <MemoryAdvanced memory={memory} />
        {memory.preview ? (
          <div>
            {memory.markdownLoading && !memory.markdown ? (
              <Skeleton className="h-28" />
            ) : (
              memory.markdown && (
                <>
                  <p className="mb-3 text-xs text-zinc-500">
                    约 {memory.markdown.estimated_tokens} tokens
                  </p>
                  {memory.markdown.markdown ? (
                    <div className="text-sm text-zinc-800">
                      <Markdown>{memory.markdown.markdown}</Markdown>
                    </div>
                  ) : (
                    enabled &&
                    memory.count === 0 && (
                      <p className="text-sm leading-6 text-zinc-500">
                        还没有记忆，多聊几句后会自动整理。
                      </p>
                    )
                  )}
                </>
              )
            )}
          </div>
        ) : (
          <div className={enabled ? undefined : 'opacity-40 grayscale'}>
            <fieldset
              className="m-0 mb-3 flex w-fit rounded-full border-0 bg-zinc-100 p-0.5 text-xs"
              aria-label="记忆状态"
            >
              {statusViews.map((view) => (
                <button
                  key={view.id}
                  type="button"
                  className={`rounded-full px-2.5 py-1 ${memory.status === view.id ? 'bg-white text-zinc-900 shadow-sm' : 'text-zinc-500'}`}
                  aria-pressed={memory.status === view.id}
                  onClick={() => memory.changeStatus(view.id)}
                >
                  {view.label}
                </button>
              ))}
            </fieldset>
            {!memory.loaded && !memory.error && <Skeleton className="h-24" />}
            {showEmpty && (
              <p className="py-6 text-center text-sm leading-6 text-zinc-500">
                {emptyCopy(memory.status)}
              </p>
            )}
            {memory.loaded && memory.items.length > 0 && (
              <>
                <MemoryGroup
                  title="关于你（所有 Agent 共享）"
                  items={userItems}
                  namespaces={namespaces}
                  hasMore={memory.hasMore}
                  enabled={enabled}
                  onChanged={memory.refresh}
                  onConflict={conflicted}
                  onRemember={kept}
                  onForget={memory.forget}
                />
                <MemoryGroup
                  title="这个 Agent 的记忆"
                  items={agentItems}
                  namespaces={namespaces}
                  hasMore={memory.hasMore}
                  enabled={enabled}
                  onChanged={memory.refresh}
                  onConflict={conflicted}
                  onRemember={kept}
                  onForget={memory.forget}
                />
              </>
            )}
            {memory.hasMore && (
              <button
                ref={sentinelRef}
                type="button"
                className="mt-4 w-full py-2 text-center text-xs text-zinc-500 underline underline-offset-4 disabled:no-underline"
                disabled={memory.loadingMore}
                onClick={() => void memory.loadMore()}
              >
                {memory.loadingMore ? '加载中…' : '加载更多'}
              </button>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
