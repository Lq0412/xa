import { ParseError } from 'eventsource-parser';
import { EventSourceParserStream } from 'eventsource-parser/stream';
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

/** 历史读取失败必须由页面显示，不能伪装为空会话。 */
export const listMessages = (id: string, signal?: AbortSignal) =>
  request<{ messages: ChatMessage[] | null }>(
    `/conversations/${encodeURIComponent(id)}/messages`,
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
    .pipeThrough(new EventSourceParserStream({ onError: 'terminate' }))
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
