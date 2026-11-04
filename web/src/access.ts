/** 界面权限与后端资源归属一致；接口仍独立执行最终授权检查。 */
export default function access(
  initialState: { currentUser?: XA.User } | undefined,
) {
  const user = initialState?.currentUser;
  return {
    canAdmin: user?.role === 'admin',
    canManageAgent: (agent: XA.Agent) =>
      Boolean(
        user &&
          agent.workspace_id === user.workspace_id &&
          (agent.owner_type === 'system'
            ? user.role === 'admin'
            : agent.owner_id === user.id),
      ),
  };
}
