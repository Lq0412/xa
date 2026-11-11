import { decideApproval as decideApprovalRequest } from './generated/approvals';

type Decision = Parameters<typeof decideApprovalRequest>[1];
export type RememberScope = NonNullable<Decision['remember']>;

/** 决定只提交给后端，授予和 Agent 版本由同一审批请求处理。 */
export const decideApproval = (id: string, body: Decision) =>
  decideApprovalRequest({ approval_id: encodeURIComponent(id) }, body);
