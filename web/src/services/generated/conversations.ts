// @ts-ignore
/* eslint-disable */
import { request } from "@/services/xa/request";

/** 列出我的会话 GET /api/v1/conversations */
export async function listConversations(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.ConversationList>("/api/v1/conversations", {
    method: "GET",
    ...(options || {}),
  });
}

/** 创建或获取会话 POST /api/v1/conversations */
export async function createConversation(
  body: {
    /** Agent ID */
    agent_id: string;
  },
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  return request<API.Conversation>("/api/v1/conversations", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 列出会话消息 GET /api/v1/conversations/${param0}/messages */
export async function listConversationMessages(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listConversationMessagesParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { conversation_id: param0, ...queryParams } = params;
  return request<API.MessagePage>(`/api/v1/conversations/${param0}/messages`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 发送消息并订阅回复 默认使用 v1 的 delta、done 和 error 事件。通过 X-XA-Stream 或 stream 选择 v2 后，事件使用运行信封，可用 Last-Event-ID 从指定序号之后继续读取。 POST /api/v1/conversations/${param0}/messages */
export async function sendConversationMessage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.sendConversationMessageParams,
  body: API.SendMessageBody,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { conversation_id: param0, ...queryParams } = params;
  return request<
    | API.StreamDelta
    | API.StreamDone
    | API.StreamError
    | { run_id: string; seq: number; step?: number; ts: string; type: string }
  >(`/api/v1/conversations/${param0}/messages`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: {
      ...queryParams,
    },
    data: body,
    ...(options || {}),
  });
}

/** 获取会话摘要 GET /api/v1/conversations/${param0}/summary */
export async function getConversationSummary(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getConversationSummaryParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { conversation_id: param0, ...queryParams } = params;
  return request<API.Summary | { summary: null }>(
    `/api/v1/conversations/${param0}/summary`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
