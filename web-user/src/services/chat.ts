import {
  type EventSourceMessage,
  EventSourceParserStream,
} from 'eventsource-parser/stream';
import type { ApprovalInfo, MessagePart } from './chat-parts';
import { applyRunEvent, type RunStreamEvent } from './chat-run';
import * as conversations from './generated/conversations';
import { cancelRun as cancelRunRequest } from './generated/runs';
import { ApiError, fetchApi } from './http';

/** 服务端保存的会话摘要，ID 同时用于路由和请求归属。 */
export type Conversation = Awaited<
  ReturnType<typeof conversations.createConversation>
>;

type StoredMessagePage = Awaited<
  ReturnType<typeof conversations.listConversationMessages>
>;
type StoredMessage = NonNullable<StoredMessagePage['messages']>[number];

/** pending 是本地生成中状态；stopped 用于本次主动停止的显示。 */
export type ChatMessage = Pick<
  StoredMessage,
  'id' | 'conversation_id' | 'role' | 'content'
> &
  Partial<Pick<StoredMessage, 'error' | 'run_id' | 'created_at'>> & {
    status: StoredMessage['status'] | 'stopped';
    parts?: MessagePart[];
  };

/** 历史 API 会剥离思考正文，流式态合并时保留本地已收到的 reasoning 文本。 */
export function mergeMessagePartsFromLocal(
  server: ChatMessage,
  local?: ChatMessage,
): ChatMessage {
  if (!server.parts?.length || !local?.parts?.length) return server;
  const parts = server.parts.map((sp) => {
    if (sp.type !== 'reasoning') return sp;
    const lp = local.parts?.find(
      (p) => p.type === 'reasoning' && p.item_id === sp.item_id,
    );
    if (
      lp?.type === 'reasoning' &&
      lp.text.trim().length > 0 &&
      !String(sp.text || '').trim()
    ) {
      return { ...sp, text: lp.text };
    }
    return sp;
  });
  return { ...server, parts };
}

/** URL 只保存稳定 ID，旧链接由会话状态层读取后转换为统一格式。 */
export function chatURL(agentId: string, conversationId?: string) {
  const query = new URLSearchParams({ agentId });
  if (conversationId) query.set('conversationId', conversationId);
  return `/chat?${query}`;
}

/** 列表只读取当前登录用户有权限访问的会话。 */
export const listConversations = (signal?: AbortSignal) =>
  conversations.listConversations({ signal });

/** 首次发送前创建真实会话，空白页面不预先写入数据库。 */
export const createConversation = (agent_id: string, signal?: AbortSignal) =>
  conversations.createConversation({ agent_id }, { signal });

/** 会话滚动摘要。没有摘要时接口返回 summary: null，这里收成 null。 */
export type ConversationSummary = Exclude<
  Awaited<ReturnType<typeof conversations.getConversationSummary>>,
  { summary: null }
>;

export async function getConversationSummary(id: string, signal?: AbortSignal) {
  const result = await conversations.getConversationSummary(
    { conversation_id: encodeURIComponent(id) },
    { signal },
  );
  if (
    result != null &&
    typeof result === 'object' &&
    'content' in result &&
    typeof result.content === 'string' &&
    typeof result.message_count === 'number' &&
    typeof result.version === 'number'
  ) {
    return result;
  }
  if (
    result != null &&
    typeof result === 'object' &&
    'summary' in result &&
    result.summary === null
  ) {
    return null;
  }
  throw new Error('对话摘要格式不正确');
}

/** 历史读取失败必须由页面显示，不能伪装为空会话。 */
type StoredActiveRun = NonNullable<StoredMessagePage['active_run']>;
export type ActiveRun = Omit<StoredActiveRun, 'pending_approvals'> & {
  pending_approvals?: (ApprovalInfo & {
    call_id: string;
    name: string;
    title: string;
  })[];
};

export async function listMessages(
  id: string,
  signal?: AbortSignal,
  before?: string,
  limit = 20,
) {
  const page = await conversations.listConversationMessages(
    {
      conversation_id: encodeURIComponent(id),
      limit: String(limit),
      before: before || undefined,
    },
    { signal },
  );
  return {
    ...page,
    // 历史和 SSE 共用按 type 区分的消息片段，生成类型是服务端的通用结构。
    active_run: page.active_run as ActiveRun | null | undefined,
    // 持久化消息沿用 failed，主动取消在界面上显示为已停止。
    messages: ((page.messages ?? []) as ChatMessage[]).map((message) =>
      message.status === 'failed' && message.error === '已停止生成'
        ? { ...message, status: 'stopped' as const }
        : message,
    ),
  };
}

/** 事件去重后，分别通知消息内容、run 标识和记忆变更。 */
export type StreamRunHandlers = {
  onParts: (parts: MessagePart[], content: string) => void;
  onRunID?: (runId: string, userMessageId?: string) => void;
  onMemoryUpdated?: () => void;
};

/** 三类终态都已写入历史，可用消息 ID 合并服务端记录。 */
export type StreamRunResult = {
  runId: string;
  messageId?: string;
  status: 'completed' | 'failed' | 'stopped';
  error?: string;
};

function readSseFrames(response: Response, onComment: () => void) {
  if (
    !response.body ||
    !response.headers.get('content-type')?.includes('text/event-stream')
  )
    throw new ApiError(502, '服务未返回有效的消息流');
  return response.body
    .pipeThrough(new TextDecoderStream())
    .pipeThrough(new EventSourceParserStream({ onComment }))
    .getReader();
}

/** 首帧前断线使用相同的幂等键重试，收到 run ID 后只订阅已有 run。 */
export function streamRun(
  conversationId: string,
  content: string,
  signal: AbortSignal,
  handlers: StreamRunHandlers,
  idempotencyKey: string,
) {
  const body: API.SendMessageBody = { content };
  return followRun(signal, handlers, (runId, afterSeq, requestSignal) =>
    runId
      ? fetchRunEvents(runId, afterSeq, requestSignal)
      : fetchApi(
          `/conversations/${encodeURIComponent(conversationId)}/messages`,
          {
            method: 'POST',
            headers: { 'X-XA-Stream': '2', 'Idempotency-Key': idempotencyKey },
            body: JSON.stringify(body),
            signal: requestSignal,
          },
        ),
  );
}

function waitForRetry(delay: number, signal: AbortSignal) {
  signal.throwIfAborted();
  return new Promise<void>((resolve, reject) => {
    const abort = () => {
      clearTimeout(timer);
      reject(signal.reason);
    };
    const timer = setTimeout(() => {
      signal.removeEventListener('abort', abort);
      resolve();
    }, delay);
    signal.addEventListener('abort', abort, { once: true });
  });
}

function parseRunEvent(
  data: string,
  event?: string,
): RunStreamEvent & { run_id: string } {
  const invalid = '回复数据格式无效';
  let value: Partial<RunStreamEvent> | null;
  try {
    value = JSON.parse(data);
  } catch (cause) {
    if (cause instanceof SyntaxError) throw new ApiError(502, invalid);
    throw cause;
  }
  if (event === 'error') {
    throw new ApiError(502, String(value?.message || '生成回复失败'));
  }
  if (
    typeof value?.seq !== 'number' ||
    !Number.isSafeInteger(value.seq) ||
    value.seq <= 0 ||
    typeof value.run_id !== 'string' ||
    !value.run_id ||
    typeof value.type !== 'string' ||
    (event && value.type !== event)
  )
    throw new ApiError(502, invalid);
  return { ...value, seq: value.seq, run_id: value.run_id, type: value.type };
}

/** 重放沿用已有 parts 和序号，连接恢复后才清除退避窗口。 */
async function followRun(
  signal: AbortSignal,
  handlers: StreamRunHandlers,
  open: (
    runId: string,
    afterSeq: number,
    signal: AbortSignal,
  ) => Promise<Response>,
  runId = '',
): Promise<StreamRunResult> {
  let afterSeq = 0;
  let parts: MessagePart[] = [];
  let content = '';
  let retryUntil = 0;
  let retryDelay = 1000;
  for (;;) {
    const connection = new AbortController();
    const requestSignal = AbortSignal.any([signal, connection.signal]);
    let deadline: ReturnType<typeof setTimeout> | undefined;
    let stable: ReturnType<typeof setTimeout> | undefined;
    let reader: ReadableStreamDefaultReader<EventSourceMessage> | undefined;
    const connected = () => {
      retryUntil = 0;
      retryDelay = 1000;
      clearTimeout(deadline);
      clearTimeout(stable);
    };
    const onComment = () => {
      if (!retryUntil) return;
      // 审批等待期间可能只有 ping，连接保持一秒后才重置重连窗口。
      clearTimeout(deadline);
      stable ??= setTimeout(connected, 1000);
    };
    const checkAbort = () => {
      signal.throwIfAborted();
      if (connection.signal.aborted) throw new ApiError(502, '连接中断');
    };
    try {
      if (retryUntil) {
        await waitForRetry(
          Math.min(retryDelay, retryUntil - performance.now()),
          signal,
        );
        const remaining = retryUntil - performance.now();
        if (remaining <= 0) throw new ApiError(502, '连接中断');
        deadline = setTimeout(() => connection.abort(), remaining);
        retryDelay = Math.min(retryDelay * 2, 4000);
      }
      let response: Response;
      try {
        response = await open(runId, afterSeq, requestSignal);
      } catch (cause) {
        checkAbort();
        if (
          !(cause instanceof TypeError) &&
          !(cause instanceof ApiError && cause.status >= 500)
        )
          throw cause;
        retryUntil ||= performance.now() + 30000;
        continue;
      }
      reader = readSseFrames(response, onComment);
      for (;;) {
        let frame: EventSourceMessage;
        try {
          const { value, done } = await reader.read();
          if (done) break;
          frame = value;
        } catch (cause) {
          checkAbort();
          if (!(cause instanceof TypeError)) throw cause;
          break;
        }
        const ev = parseRunEvent(frame.data, frame.event);
        if (runId && ev.run_id !== runId)
          throw new ApiError(502, '回复的运行标识不一致');
        if (ev.seq <= afterSeq) continue;
        afterSeq = ev.seq;
        connected();
        if (!runId) {
          runId = ev.run_id;
          const userMessageId =
            typeof ev.user_message_id === 'string'
              ? ev.user_message_id
              : undefined;
          handlers.onRunID?.(runId, userMessageId);
        }
        if (ev.type === 'memory.updated') handlers.onMemoryUpdated?.();
        if (ev.type === 'message.delta') content += String(ev.delta ?? '');
        parts = applyRunEvent(parts, ev);
        handlers.onParts(parts, content);
        const messageId =
          typeof ev.message_id === 'string' ? ev.message_id : undefined;
        if (ev.type === 'run.completed')
          return { messageId, runId, status: 'completed' };
        if (ev.type === 'run.cancelled')
          return { messageId, runId, status: 'stopped' };
        if (ev.type === 'run.failed')
          return {
            messageId,
            runId,
            status: 'failed',
            error: String(ev.message || '生成回复失败'),
          };
      }
    } finally {
      clearTimeout(deadline);
      clearTimeout(stable);
      connection.abort();
      reader?.releaseLock();
    }
    retryUntil ||= performance.now() + 30000;
  }
}

/** 服务端已终止的 run 返回原终态，仍在执行的 run 返回 cancelling。 */
export function cancelRun(runId: string, signal?: AbortSignal) {
  return cancelRunRequest({ run_id: encodeURIComponent(runId) }, { signal });
}

/** 刷新后从头重放，补齐尚未保存到历史的消息过程。 */
export function resumeRun(
  runId: string,
  signal: AbortSignal,
  handlers: StreamRunHandlers,
) {
  return followRun(signal, handlers, fetchRunEvents, runId);
}

function fetchRunEvents(runId: string, afterSeq: number, signal: AbortSignal) {
  return fetchApi(
    `/runs/${encodeURIComponent(runId)}/events?after=${afterSeq}`,
    {
      signal,
      headers: afterSeq > 0 ? { 'Last-Event-ID': String(afterSeq) } : {},
    },
  );
}
