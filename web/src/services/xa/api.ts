import * as agents from '../generated/agents';
import { listAdminApprovals } from '../generated/approvals';
import * as auth from '../generated/auth';
import * as tools from '../generated/tools';

/** 登录失败交给表单展示，不触发会话过期跳转。 */
export async function login(email: string, password: string) {
  return auth.login({ email, password }, { anonymous: true });
}

/** 注册接口只创建普通用户，角色由服务端确定。 */
export async function register(
  email: string,
  password: string,
  displayName: string,
) {
  return auth.register(
    { email, password, display_name: displayName },
    { anonymous: true },
  );
}

/** 撤销当前 Redis 会话；成功后才能清除本地登录状态。 */
export async function logout(): Promise<void> {
  await auth.logout();
}

/** 刷新页面时向服务端核对令牌和用户身份。 */
export async function getCurrentUser() {
  return auth.getMe();
}

/** 获取当前用户有权浏览的公共和私有 Agent。 */
export async function listAgents(signal?: AbortSignal) {
  return agents.listAgents({ signal });
}

/** 读取编辑时的最新元数据，避免直接沿用列表缓存。 */
export async function getAgent(agentId: string) {
  return agents.getAgent({ agent_id: encodeURIComponent(agentId) });
}

/** 获取完整配置；编辑器必须保留暂未展示的绑定和策略。 */
export async function getAgentConfig(agentId: string) {
  return agents.getAgentConfig({ agent_id: encodeURIComponent(agentId) });
}

/** 与后端保存接口一致的字段；头像和完整配置不能因表单省略而丢失。 */
export type AgentSaveInput = Required<
  Pick<
    API.AgentRequest,
    'name' | 'avatar' | 'description' | 'visibility' | 'configuration'
  >
> & { tags: NonNullable<API.AgentRequest['tags']> };

/** 首次保存创建 Agent 及其首个配置版本。 */
export async function createAgent(data: AgentSaveInput) {
  return agents.createAgent(data);
}

/** 传递读取时的版本号，过期编辑由后端返回 409，禁止覆盖他人的修改。 */
export async function saveAgent(
  agentId: string,
  expectedVersionId: string,
  data: AgentSaveInput,
) {
  return agents.saveAgent(
    { agent_id: encodeURIComponent(agentId) },
    { ...data, expected_version_id: expectedVersionId },
  );
}

/** 启停操作使用后端权限与状态校验。 */
export async function setAgentStatus(agentId: string, enabled: boolean) {
  const params = { agent_id: encodeURIComponent(agentId) };
  await (enabled ? agents.enableAgent(params) : agents.offlineAgent(params));
}

/** 通过服务端执行软删除，页面仅在成功后刷新。 */
export async function deleteAgent(agentId: string): Promise<void> {
  await agents.deleteAgent({ agent_id: encodeURIComponent(agentId) });
}

/** 管理端的审批摘要不包含工具原始参数。 */
export type ApprovalRecord = NonNullable<
  Awaited<ReturnType<typeof listAdminApprovals>>['approvals']
>[number];

/** 由服务端筛选并返回下一页游标。 */
export function listApprovals(
  filters: {
    status: NonNullable<API.listAdminApprovalsParams['status']> | '';
    tool_name: string;
    cursor: string;
  },
  signal?: AbortSignal,
) {
  return listAdminApprovals(
    {
      limit: '50',
      status: filters.status || undefined,
      tool_name: filters.tool_name || undefined,
      cursor: filters.cursor || undefined,
    },
    { signal },
  );
}

/** 目录可供管理员读取，公共 Agent 不调用仅限所有者的工具视图接口。 */
export type CatalogTool = NonNullable<
  Awaited<ReturnType<typeof tools.listTools>>['tools']
>[number];

export function listTools() {
  return tools.listTools();
}

/** 概览统计遍历全部游标，不能把第一页的条数当成总数。 */
export async function allApprovals(signal: AbortSignal) {
  const rows: ApprovalRecord[] = [];
  let cursor = '';
  do {
    signal.throwIfAborted();
    const result = await listApprovals(
      { status: '', tool_name: '', cursor },
      signal,
    );
    rows.push(...result.approvals);
    cursor = result.next_cursor;
  } while (cursor);
  return rows;
}
