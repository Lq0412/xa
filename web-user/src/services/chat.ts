import { ParseError } from 'eventsource-parser';
import { EventSourceParserStream } from 'eventsource-parser/stream';
import type { MessagePart } from './chat-parts';
import { applyRunEvent, type RunStreamEvent } from './chat-run';
import { ApiError, fetchApi, request } from './http';

/** 服务端保存的会话摘要，ID 同时用于路由和请求归属。 */
export type Conversation = {
  id: string;
  agent_id: string;
  title: string;
  updated_at: string;
};

/** pending 是本地生成中状态；stopped 用于本次主动停止的显示。 */
export type ChatMessage = {
  id: string;
  conversation_id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  status: 'pending' | 'completed' | 'failed' | 'stopped';
  error?: string;
  run_id?: string;
  parts?: MessagePart[];
  created_at?: string;
};

/** URL 只保存稳定 ID，旧链接由会话状态层读取后转换为统一格式。 */
export function chatURL(agentId: string, conversationId?: string) {
  const query = new URLSearchParams({ agentId });
  if (conversationId) query.set('conversationId', conversationId);
  return `/chat?${query}`;
}

/** 列表只读取当前登录用户有权限访问的会话。 */
export const listConversations = (signal?: AbortSignal) =>
  request<{ conversations: Conversation[] | null }>('/conversations', {
    signal,
  });

/** 首次发送前创建真实会话，空白页面不预先写入数据库。 */
export const createConversation = (agent_id: string, signal?: AbortSignal) =>
  request<Conversation>('/conversations', {
    method: 'POST',
    body: JSON.stringify({ agent_id }),
    signal,
  });

/** 会话滚动摘要。没有摘要时接口返回 summary: null，这里收成 null。 */
export type ConversationSummary = {
  id: string;
  conversation_id: string;
  content: string;
  covers_from_message_id: string | null;
  covers_until_message_id: string | null;
  message_count: number;
  estimated_tokens: number;
  version: number;
  created_at: string;
};

export async function getConversationSummary(id: string, signal?: AbortSignal) {
  const result = await request<ConversationSummary | { summary: null }>(
    `/conversations/${encodeURIComponent(id)}/summary`,
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
export type ActiveRun = {
  id: string;
  status: string;
  last_event_seq: number;
};

export const listMessages = (
  id: string,
  signal?: AbortSignal,
  before?: string,
  limit = 20,
) =>
  request<{
    messages: ChatMessage[];
    has_more: boolean;
    before?: string;
    active_run?: ActiveRun | null;
  }>(
    `/conversations/${encodeURIComponent(id)}/messages?limit=${limit}${before ? `&before=${encodeURIComponent(before)}` : ''}`,
    { signal },
  );

/** 用标准 SSE 解析器处理分片；只有 done 事件代表完整成功。 */
export async function streamMessage(
  id: string,
  content: string,
  signal: AbortSignal,
  onDelta: (value: string) => void,
): Promise<string> {
  const cleanup = new AbortController();
  const response = await fetchApi(
    `/conversations/${encodeURIComponent(id)}/messages`,
    {
      method: 'POST',
      body: JSON.stringify({ content }),
      signal: AbortSignal.any([signal, cleanup.signal]),
    },
  );
  if (
    !response.body ||
    !response.headers.get('content-type')?.includes('text/event-stream')
  )
    throw new ApiError(502, '服务未返回有效的消息流');
  const reader = response.body
    .pipeThrough(new TextDecoderStream())
    .pipeThrough(
      new EventSourceParserStream({
        onError: () => {
          /* 分片中间态由解析器自行恢复 */
        },
      }),
    )
    .getReader();
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done)
        throw new ApiError(502, '回复连接提前结束，请重新加载记录确认结果');
      const data = JSON.parse(value.data);
      if (value.event === 'error')
        throw new ApiError(502, data.message || '生成回复失败');
      if (value.event === 'delta') {
        if (typeof data.delta !== 'string')
          throw new ApiError(502, '回复内容格式无效');
        onDelta(data.delta);
      } else if (value.event === 'done') {
        if (typeof data.message_id !== 'string')
          throw new ApiError(502, '回复缺少消息标识');
        return data.message_id;
      }
    }
  } catch (cause) {
    if (cause instanceof SyntaxError || cause instanceof ParseError)
      throw new ApiError(502, '回复数据格式无效，请重新加载记录确认结果');
    throw cause;
  } finally {
    cleanup.abort();
    reader.releaseLock();
  }
}

export type StreamRunHandlers = {
  onParts: (parts: MessagePart[], content: string) => void;
  onRunID?: (runId: string) => void;
};

type SseFrame = { event: string; data: string };

async function* readSseFrames(
  body: ReadableStream<Uint8Array>,
): AsyncGenerator<SseFrame> {
  const reader = (body as ReadableStream<Uint8Array<ArrayBuffer>>)
    .pipeThrough(new TextDecoderStream())
    .getReader();
  let buf = '';
  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;
      buf += value;
      for (;;) {
        const sep = buf.indexOf('\n\n');
        if (sep < 0) break;
        const block = buf.slice(0, sep);
        buf = buf.slice(sep + 2);
        if (block.startsWith(':')) continue;
        let event = '';
        let data = '';
        for (const line of block.split('\n')) {
          if (line.startsWith('event:')) event = line.slice(6).trim();
          else if (line.startsWith('data:')) data += line.slice(5).trim();
        }
        if (data) yield { event, data };
      }
    }
  } finally {
    reader.releaseLock();
  }
}

/** SSE v2：X-XA-Stream: 2，支持取消与续传由调用方处理 run_id。 */
export async function streamRun(
  conversationId: string,
  content: string,
  signal: AbortSignal,
  handlers: StreamRunHandlers,
  opts?: { idempotencyKey?: string; afterSeq?: number },
): Promise<{ messageId?: string; runId?: string }> {
  const headers: Record<string, string> = { 'X-XA-Stream': '2' };
  if (opts?.idempotencyKey) headers['Idempotency-Key'] = opts.idempotencyKey;
  if (opts?.afterSeq != null && opts.afterSeq > 0)
    headers['Last-Event-ID'] = String(opts.afterSeq);
  const response = await fetchApi(
    `/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      method: 'POST',
      headers,
      body: JSON.stringify({ content }),
      signal,
    },
  );
  if (
    !response.body ||
    !response.headers.get('content-type')?.includes('text/event-stream')
  )
    throw new ApiError(502, '服务未返回有效的消息流');
  let parts: MessagePart[] = [];
  let contentText = '';
  let runId = '';
  let messageId = '';
  for await (const frame of readSseFrames(response.body)) {
    const ev = JSON.parse(frame.data) as RunStreamEvent;
    const typ = String(frame.event || ev.type || '');
    if (typeof ev.run_id === 'string') {
      runId = ev.run_id;
      handlers.onRunID?.(runId);
    }
    if (typ === 'message.delta') {
      contentText += String(ev.delta ?? '');
    }
    parts = applyRunEvent(parts, { ...ev, type: typ });
    handlers.onParts(parts, contentText);
    if (
      typ === 'run.completed' ||
      typ === 'run.failed' ||
      typ === 'run.cancelled'
    ) {
      messageId = String(ev.message_id ?? '');
      if (typ !== 'run.completed')
        throw new ApiError(502, String(ev.message ?? '生成失败'));
      break;
    }
  }
  return { messageId, runId };
}

export async function cancelRun(runId: string, signal?: AbortSignal) {
  await fetchApi(`/runs/${encodeURIComponent(runId)}/cancel`, {
    method: 'POST',
    signal,
  });
}

export async function resumeRun(
  runId: string,
  afterSeq: number,
  signal: AbortSignal,
  handlers: StreamRunHandlers,
) {
  const response = await fetchApi(
    `/runs/${encodeURIComponent(runId)}/events?after=${afterSeq}`,
    {
      signal,
      headers: afterSeq > 0 ? { 'Last-Event-ID': String(afterSeq) } : {},
    },
  );
  if (!response.body) throw new ApiError(502, '续传失败');
  let parts: MessagePart[] = [];
  let contentText = '';
  for await (const frame of readSseFrames(response.body)) {
    const ev = JSON.parse(frame.data) as RunStreamEvent;
    const typ = String(frame.event || ev.type || '');
    if (typ === 'message.delta') contentText += String(ev.delta ?? '');
    parts = applyRunEvent(parts, { ...ev, type: typ });
    handlers.onParts(parts, contentText);
    if (
      typ === 'run.completed' ||
      typ === 'run.failed' ||
      typ === 'run.cancelled'
    ) {
      if (typ !== 'run.completed')
        throw new ApiError(502, String(ev.message ?? '生成失败'));
      break;
    }
  }
}
