// @ts-ignore
/* eslint-disable */
import { request } from "@/services/openapi-request";

/** 列出工作空间运行 GET /api/v1/admin/runs */
export async function listAdminRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminRunsParams,
  options?: { [key: string]: any }
) {
  return request<API.RunList>("/api/v1/admin/runs", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 获取运行详情 GET /api/v1/admin/runs/${param0} */
export async function getAdminRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAdminRunParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.RunDetail>(`/api/v1/admin/runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取运行执行树 GET /api/v1/admin/runs/${param0}/spans */
export async function getAdminRunSpans(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAdminRunSpansParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ run_id: string; span: API.SpanNode | null }>(
    `/api/v1/admin/runs/${param0}/spans`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 列出 Agent 活动 GET /api/v1/agents/${param0}/activity */
export async function getAgentActivity(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentActivityParams,
  options?: { [key: string]: any }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.ActivityPage>(`/api/v1/agents/${param0}/activity`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 列出我的运行 GET /api/v1/runs */
export async function listRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listRunsParams,
  options?: { [key: string]: any }
) {
  return request<API.RunList>("/api/v1/runs", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 获取运行详情 GET /api/v1/runs/${param0} */
export async function getRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRunParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.RunDetail>(`/api/v1/runs/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 取消运行 POST /api/v1/runs/${param0}/cancel */
export async function cancelRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelRunParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, ...queryParams } = params;
  return request<API.CancelResult>(`/api/v1/runs/${param0}/cancel`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 订阅运行事件 使用 v2 事件信封。读取 after 与有效 Last-Event-ID 两者中较大的序号之后的事件，连接可收到 SSE 注释心跳。 GET /api/v1/runs/${param0}/events */
export async function subscribeRunEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.subscribeRunEventsParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, ...queryParams } = params;
  return request<
    | { run_id: string; seq: number; step?: number; ts: string; type: string }
    | API.StreamError
  >(`/api/v1/runs/${param0}/events`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 获取运行执行树 GET /api/v1/runs/${param0}/spans */
export async function getRunSpans(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRunSpansParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, ...queryParams } = params;
  return request<{ run_id: string; span: API.SpanNode | null }>(
    `/api/v1/runs/${param0}/spans`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 获取工具调用结果 GET /api/v1/runs/${param0}/tool-results/${param1} */
export async function getToolResult(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getToolResultParams,
  options?: { [key: string]: any }
) {
  const { run_id: param0, call_id: param1, ...queryParams } = params;
  return request<API.ToolResultDetail>(
    `/api/v1/runs/${param0}/tool-results/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}
