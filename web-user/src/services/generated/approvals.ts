// @ts-ignore
/* eslint-disable */
import { request } from "@/services/openapi-request";

/** 列出工作空间审批记录 GET /api/v1/admin/approvals */
export async function listAdminApprovals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminApprovalsParams,
  options?: { [key: string]: any }
) {
  return request<API.ApprovalRecordsPage>("/api/v1/admin/approvals", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 列出我的审批 GET /api/v1/approvals */
export async function listUserApprovals(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listUserApprovalsParams,
  options?: { [key: string]: any }
) {
  return request<API.UserApprovalPage>("/api/v1/approvals", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 允许或拒绝工具执行 POST /api/v1/approvals/${param0}/decision */
export async function decideApproval(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.decideApprovalParams,
  body: API.DecisionBody,
  options?: { [key: string]: any }
) {
  const { approval_id: param0, ...queryParams } = params;
  return request<API.DecisionResult>(`/api/v1/approvals/${param0}/decision`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}
