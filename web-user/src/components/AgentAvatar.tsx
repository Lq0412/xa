import type { Agent } from '@/services/api';

/** 只展示真实头像；没有头像时使用名称首字，不请求虚构的图片资源。 */
export function AgentAvatar({
  agent,
  large = false,
}: {
  agent: Agent;
  large?: boolean;
}) {
  return (
    <span className={`agent-avatar ${large ? 'large' : ''}`} aria-hidden="true">
      {agent.avatar ? (
        <img src={agent.avatar} alt="" />
      ) : (
        Array.from(agent.name)[0]
      )}
    </span>
  );
}
