// @ts-ignore
/* eslint-disable */
import { request } from "@/services/xa/request";

/** 获取 Agent 当前工具配置 GET /api/v1/agents/${param0}/tools */
export async function getAgentTools(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentToolsParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.AgentToolsResponse>(`/api/v1/agents/${param0}/tools`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 更新 Agent 工具配置 PUT /api/v1/agents/${param0}/tools */
export async function updateAgentTools(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateAgentToolsParams,
  body: API.UpdateAgentToolsRequest,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.AgentToolsResponse>(`/api/v1/agents/${param0}/tools`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 获取平台工具目录 GET /api/v1/tools */
export async function listTools(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.ToolCatalogResponse>("/api/v1/tools", {
    method: "GET",
    ...(options || {}),
  });
}
