import { history, Link, useLocation, useModel } from '@umijs/max';
import { useEffect, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { AgentEditor } from '@/components/AgentEditor';
import { Icon } from '@/components/Icon';
import {
  type Agent,
  addPublicAgent,
  errorMessage,
  listAgentLibrary,
  listAgents,
} from '@/services/api';
import { chatURL } from '@/services/chat';

/** 发现与我的 Agent 只从图标栏进入，数据与对话页共享。 */
export default function AgentsPage() {
  const mine = useLocation().pathname === '/my-agents';
  const { initialState } = useModel('@@initialState');
  const user = initialState?.currentUser;
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [query, setQuery] = useState('');
  const [revision, setRevision] = useState(0);
  const [creating, setCreating] = useState(false);
  const [adding, setAdding] = useState<string>();
  const [added, setAdded] = useState(new Set<string>());
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 表示保存或重试后的显式读取。
  useEffect(() => {
    let disposed = false;
    setLoading(true);
    setError('');
    Promise.all([
      listAgents(mine),
      user ? listAgentLibrary() : Promise.resolve({ agents: [] }),
    ])
      .then(([result, library]) => {
        if (!disposed) {
          setAdded(new Set(library.agents.map((agent) => agent.id)));
          setAgents(
            (result.agents ?? []).filter(
              (agent) =>
                !mine ||
                (agent.owner_id === user?.id && agent.visibility === 'private'),
            ),
          );
        }
      })
      .catch((cause) => {
        if (!disposed) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!disposed) setLoading(false);
      });
    return () => {
      disposed = true;
    };
  }, [mine, user?.id, revision]);
  const visible = agents.filter((agent) =>
    `${agent.name} ${agent.description} ${(agent.tags ?? []).join(' ')}`
      .toLocaleLowerCase()
      .includes(query.trim().toLocaleLowerCase()),
  );
  const openCreate = () => {
    if (!user) history.push('/login?redirect=/my-agents');
    else setCreating(true);
  };
  const add = async (agent: Agent) => {
    if (!user) {
      history.push(`/login?redirect=${encodeURIComponent(chatURL(agent.id))}`);
      return;
    }
    setAdding(agent.id);
    try {
      await addPublicAgent(agent.id);
      history.push(chatURL(agent.id));
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setAdding(undefined);
    }
  };
  return (
    <div className="page-content agent-directory">
      <header className="page-heading">
        <h1>{mine ? '我的 Agent' : '发现 Agent'}</h1>
        <button type="button" className="button primary" onClick={openCreate}>
          <Icon name="plus" size={16} />
          新建 Agent
        </button>
      </header>
      <div className="directory-toolbar">
        <label className="search-field">
          <Icon name="search" size={17} />
          <input
            aria-label="搜索 Agent"
            placeholder="搜索 Agent"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>
      </div>
      {error ? (
        <div role="alert" className="error-banner">
          {error}
          <button
            type="button"
            className="text-button"
            onClick={() => setRevision((value) => value + 1)}
          >
            重试
          </button>
        </div>
      ) : loading ? (
        <p className="muted" role="status">
          正在读取 Agent…
        </p>
      ) : visible.length ? (
        <div className="agent-grid">
          {visible.map((agent) => (
            <article key={agent.id} className="agent-card">
              <div className="agent-card-top">
                <AgentAvatar agent={agent} />
                <span className="muted">
                  {agent.is_primary
                    ? '主 Agent'
                    : agent.visibility === 'public'
                      ? '公共'
                      : '私有'}
                </span>
              </div>
              <h2>{agent.name}</h2>
              {agent.description && <p>{agent.description}</p>}
              <div className="agent-tags">
                {(agent.tags ?? []).map((tag) => (
                  <span key={tag}>{tag}</span>
                ))}
              </div>
              <div className="agent-card-bottom">
                {agent.status === 'offline' ? (
                  <span>已停用</span>
                ) : agent.visibility === 'public' && !added.has(agent.id) ? (
                  <button
                    type="button"
                    className="button quiet"
                    disabled={Boolean(adding)}
                    onClick={() => void add(agent)}
                  >
                    {adding === agent.id ? '添加中…' : '添加并对话'}
                  </button>
                ) : (
                  <Link to={chatURL(agent.id)}>
                    进入对话 <Icon name="arrow-right" size={16} />
                  </Link>
                )}
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="directory-empty">
          <h2>
            {query
              ? '没有匹配的 Agent'
              : mine
                ? '还没有自己的 Agent'
                : '暂无公共 Agent'}
          </h2>
          <button
            type="button"
            className="button"
            onClick={query ? () => setQuery('') : openCreate}
          >
            {query ? '清除搜索' : '新建 Agent'}
          </button>
        </div>
      )}
      {creating && (
        <AgentEditor
          onCancel={() => setCreating(false)}
          onSaved={(agent) => {
            setCreating(false);
            history.push(chatURL(agent.id));
          }}
        />
      )}
    </div>
  );
}
