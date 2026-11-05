import { useMemo, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { Icon } from '@/components/Icon';
import type { Agent } from '@/services/api';

type Props = {
  agents: Agent[];
  activeAgentID?: string;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onCreate: () => void;
  onSelectAgent: (agent: Agent) => void;
};

/** 只保留可对话 Agent；会话由 Agent 唯一对话承载，不在侧栏重复展开。 */
export function AgentSidebar({
  agents,
  activeAgentID,
  loading,
  error,
  onRetry,
  onCreate,
  onSelectAgent,
}: Props) {
  const [search, setSearch] = useState('');
  const filtered = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase();
    return agents.filter(
      (agent) => !keyword || agent.name.toLocaleLowerCase().includes(keyword),
    );
  }, [agents, search]);
  return (
    <div className="agent-sidebar-content">
      <div className="agent-sidebar-toolbar">
        <strong>Agent</strong>
        <button
          type="button"
          className="sidebar-add-button"
          onClick={onCreate}
          aria-label="新建 Agent"
        >
          <Icon name="plus" size={18} />
        </button>
      </div>
      <label className="sidebar-search">
        <Icon name="search" size={16} />
        <input
          aria-label="搜索 Agent"
          placeholder="搜索 Agent"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
      </label>
      <nav className="agent-sidebar-list" aria-label="可对话 Agent">
        {loading ? (
          <div className="sidebar-state">正在读取</div>
        ) : error ? (
          <div className="sidebar-state">
            <p>{error}</p>
            <button type="button" className="text-button" onClick={onRetry}>
              重试
            </button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="sidebar-state">没有 Agent</div>
        ) : (
          filtered.map((agent) => (
            <button
              key={agent.id}
              type="button"
              aria-label={agent.name}
              className={`agent-sidebar-select ${agent.id === activeAgentID ? 'active' : ''}`}
              aria-current={agent.id === activeAgentID ? 'page' : undefined}
              onClick={() => onSelectAgent(agent)}
            >
              <AgentAvatar agent={agent} />
              <span className="agent-sidebar-name">{agent.name}</span>
              {agent.is_primary && (
                <span className="primary-agent-badge">主 Agent</span>
              )}
            </button>
          ))
        )}
      </nav>
    </div>
  );
}
