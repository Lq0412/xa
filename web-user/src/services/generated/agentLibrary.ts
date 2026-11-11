// @ts-ignore
/* eslint-disable */
import { request } from "@/services/openapi-request";

/** 获取用户 Agent 库 GET /api/v1/agent-library */
export async function listAgentLibrary(options?: { [key: string]: any }) {
  return request<API.LibraryListResponse>("/api/v1/agent-library", {
    method: "GET",
    ...(options || {}),
  });
}

/** 添加公开 Agent 到用户库 PUT /api/v1/agent-library/${param0} */
export async function addAgentToLibrary(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.addAgentToLibraryParams,
  options?: { [key: string]: any }
) {
  const { agent_id: param0, ...queryParams } = params;
  return request<any>(`/api/v1/agent-library/${param0}`, {
    method: "PUT",
    params: { ...queryParams },
    ...(options || {}),
  });
}
