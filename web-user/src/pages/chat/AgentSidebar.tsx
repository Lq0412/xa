import {
  DownOutlined,
  PlusOutlined,
  RightOutlined,
  RobotOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { Alert, Avatar, Button, Input } from 'antd';
import { useEffect, useState } from 'react';
import type { Agent } from '@/services/api';
import type { Conversation } from '@/services/chat';

type Props = {
  agents: Agent[];
  conversations: Conversation[];
  activeAgentID?: string | null;
  activeConversationID?: string | null;
  loading: boolean;
  historyLoading: boolean;
  error: string;
  onRetry: () => void;
  onCreate: () => void;
  onSelectAgent: (agent: Agent) => void;
  onSelectConversation: (id: string) => void;
  onNewConversation: (agent: Agent) => void;
};

/** 每个 Agent 只管理自己的历史展开状态，选择由 URL 和会话状态层决定。 */
export function AgentSidebar({
  agents,
  conversations,
  activeAgentID,
  activeConversationID,
  loading,
  historyLoading,
  error,
  onRetry,
  onCreate,
  onSelectAgent,
  onSelectConversation,
  onNewConversation,
}: Props) {
  const [query, setQuery] = useState('');
  const [expanded, setExpanded] = useState(new Set<string>());
  useEffect(() => {
    if (activeAgentID)
      setExpanded((current) => new Set(current).add(activeAgentID));
  }, [activeAgentID]);
  const visible = agents.filter((agent) =>
    agent.name.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()),
  );
  return (
    <div className="agent-sidebar-content">
      <Button type="primary" icon={<PlusOutlined />} block onClick={onCreate}>
        新建 Agent
      </Button>
      <Input
        aria-label="搜索对话 Agent"
        placeholder="搜索 Agent"
        prefix={<SearchOutlined />}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        allowClear
      />
      {error ? (
        <Alert
          type="error"
          title={error}
          action={
            <Button size="small" onClick={onRetry}>
              重试
            </Button>
          }
        />
      ) : (
        <nav className="agent-sidebar-list" aria-label="Agent 列表">
          {loading && !agents.length ? (
            <p className="sidebar-state" role="status">
              正在加载…
            </p>
          ) : !visible.length ? (
            <p className="sidebar-state">
              {query ? '没有匹配的 Agent' : '暂无 Agent'}
            </p>
          ) : (
            visible.map((agent) => {
              const open = expanded.has(agent.id);
              const history = conversations.filter(
                (item) => item.agent_id === agent.id,
              );
              return (
                <section className="agent-sidebar-group" key={agent.id}>
                  <div
                    className={`agent-sidebar-row ${agent.id === activeAgentID ? 'active' : ''}`}
                  >
                    <button
                      type="button"
                      className="agent-sidebar-select"
                      aria-label={agent.name}
                      aria-description={
                        agent.is_default ? '主 Agent' : undefined
                      }
                      aria-current={
                        agent.id === activeAgentID ? 'page' : undefined
                      }
                      onClick={() => onSelectAgent(agent)}
                      title={agent.name}
                    >
                      <Avatar
                        size={32}
                        src={agent.avatar || undefined}
                        icon={<RobotOutlined />}
                      />
                      <span className="agent-sidebar-name">{agent.name}</span>
                      {agent.is_default && (
                        <span className="primary-agent-badge">主 Agent</span>
                      )}
                    </button>
                    <button
                      type="button"
                      className="agent-history-toggle"
                      aria-label={`${open ? '收起' : '展开'} ${agent.name} 的会话`}
                      aria-expanded={open}
                      aria-controls={`history-${agent.id}`}
                      onClick={() =>
                        setExpanded((current) => {
                          const next = new Set(current);
                          if (next.has(agent.id)) next.delete(agent.id);
                          else next.add(agent.id);
                          return next;
                        })
                      }
                    >
                      {open ? <DownOutlined /> : <RightOutlined />}
                    </button>
                  </div>
                  <div
                    id={`history-${agent.id}`}
                    className="agent-history"
                    hidden={!open}
                  >
                    <button
                      type="button"
                      className="agent-new-conversation"
                      aria-label={`与 ${agent.name} 新建会话`}
                      onClick={() => onNewConversation(agent)}
                    >
                      <PlusOutlined />
                      新建会话
                    </button>
                    {historyLoading && !history.length ? (
                      <span className="sidebar-state">正在读取会话…</span>
                    ) : (
                      history.map((item) => (
                        <button
                          key={item.id}
                          type="button"
                          className={`conversation-item ${item.id === activeConversationID ? 'active' : ''}`}
                          aria-current={
                            item.id === activeConversationID
                              ? 'page'
                              : undefined
                          }
                          title={item.title || '新对话'}
                          onClick={() => onSelectConversation(item.id)}
                        >
                          <span className="conversation-item-title">
                            {item.title || '新对话'}
                          </span>
                        </button>
                      ))
                    )}
                  </div>
                </section>
              );
            })
          )}
        </nav>
      )}
    </div>
  );
}
