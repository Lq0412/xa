import { request } from './http';

export type RememberScope = 'once' | 'conversation' | 'agent';

/** 决定只提交给后端，授予和 Agent 版本由同一审批请求处理。 */
export const decideApproval = (
  id: string,
  body: {
    decision: 'approve' | 'reject';
    remember: RememberScope;
    reason?: string;
  },
) =>
  request<{ status: string; warning?: string }>(
    `/approvals/${encodeURIComponent(id)}/decision`,
    { method: 'POST', body: JSON.stringify(body) },
  );
