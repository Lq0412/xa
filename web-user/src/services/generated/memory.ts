// @ts-ignore
/* eslint-disable */
import { request } from "@/services/openapi-request";

/** 获取 Agent 的核心记忆 GET /api/v1/agents/${param0}/memory */
export async function getAgentMemory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentMemoryParams,
  options?: { [key: string]: any }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.View>(`/api/v1/agents/${param0}/memory`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 更新 Agent 记忆策略 PUT /api/v1/agents/${param0}/memory-policy */
export async function updateMemoryPolicy(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateMemoryPolicyParams,
  body: API.MemoryPolicyRequest,
  options?: { [key: string]: any }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<API.MemoryPolicyResponse>(
    `/api/v1/agents/${param0}/memory-policy`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** 列出当前用户的记忆 GET /api/v1/memories */
export async function listMemories(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMemoriesParams,
  options?: { [key: string]: any }
) {
  return request<API.Page>("/api/v1/memories", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 新增手动记忆 POST /api/v1/memories */
export async function addMemory(
  body: API.AddMemoryRequest,
  options?: { [key: string]: any }
) {
  return request<API.Item>("/api/v1/memories", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 编辑记忆并保留原条目 PUT /api/v1/memories/${param0} */
export async function editMemory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.editMemoryParams,
  body: API.EditInput,
  options?: { [key: string]: any }
) {
  const { memory_id: param0, ...queryParams } = params;
  return request<API.Item>(`/api/v1/memories/${param0}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 删除记忆 DELETE /api/v1/memories/${param0} */
export async function deleteMemory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteMemoryParams,
  options?: { [key: string]: any }
) {
  const { memory_id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/memories/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取记忆取代链审计 GET /api/v1/memories/${param0}/events */
export async function listMemoryEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listMemoryEventsParams,
  options?: { [key: string]: any }
) {
  const { memory_id: param0, ...queryParams } = params;
  return request<API.MemoryEventsResponse>(
    `/api/v1/memories/${param0}/events`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** 恢复记忆并保留原条目 POST /api/v1/memories/${param0}/restore */
export async function restoreMemory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreMemoryParams,
  options?: { [key: string]: any }
) {
  const { memory_id: param0, ...queryParams } = params;
  return request<API.Item>(`/api/v1/memories/${param0}/restore`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 重新入队已终止的记忆任务 POST /api/v1/memory/jobs/${param0}/retry */
export async function retryMemoryJob(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryMemoryJobParams,
  options?: { [key: string]: any }
) {
  const { job_id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/memory/jobs/${param0}/retry`, {
    method: "POST",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 统计当前用户的记忆和任务 GET /api/v1/memory/stats */
export async function getMemoryStats(options?: { [key: string]: any }) {
  return request<API.Stats>("/api/v1/memory/stats", {
    method: "GET",
    ...(options || {}),
  });
}
