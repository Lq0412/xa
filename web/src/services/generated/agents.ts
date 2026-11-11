// @ts-ignore
/* eslint-disable */
import { request } from "@/services/xa/request";

/** 列出 Agent GET /api/v1/agents */
export async function listAgents(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.AgentListResponse>("/api/v1/agents", {
    method: "GET",
    ...(options || {}),
  });
}

/** 创建 Agent POST /api/v1/agents */
export async function createAgent(
  body: API.AgentRequest,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  return request<API.AgentSaveResponse>("/api/v1/agents", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 获取 Agent GET /api/v1/agents/${param0} */
export async function getAgent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.AgentDefinition>(`/api/v1/agents/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除 Agent DELETE /api/v1/agents/${param0} */
export async function deleteAgent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteAgentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/agents/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取 Agent 当前配置 GET /api/v1/agents/${param0}/config */
export async function getAgentConfig(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentConfigParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.AgentConfigResponse>(`/api/v1/agents/${param0}/config`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 保存 Agent 配置 PUT /api/v1/agents/${param0}/config */
export async function saveAgent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveAgentParams,
  body: API.AgentRequest,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.AgentSaveResponse>(`/api/v1/agents/${param0}/config`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 启用 Agent POST /api/v1/agents/${param0}/enable */
export async function enableAgent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.enableAgentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/agents/${param0}/enable`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 下线 Agent POST /api/v1/agents/${param0}/offline */
export async function offlineAgent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.offlineAgentParams,
  options?: { signal?: AbortSignal; anonymous?: boolean }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/agents/${param0}/offline`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取公开可用 Agent GET /api/v1/available-agents */
export async function availableAgents(options?: {
  signal?: AbortSignal;
  anonymous?: boolean;
}) {
  return request<API.AgentListResponse>("/api/v1/available-agents", {
    method: "GET",
    ...(options || {}),
  });
}
