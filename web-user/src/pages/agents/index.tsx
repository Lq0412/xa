import {
  ArrowRightOutlined,
  PlusOutlined,
  RobotOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { history, Link, useLocation, useModel } from '@umijs/max';
import { Alert, Avatar, Button, Empty, Input, Skeleton, Tag } from 'antd';
import { useEffect, useState } from 'react';
import { AgentEditor } from '@/components/AgentEditor';
import {
  type Agent,
  addPublicAgent,
  errorMessage,
  listAgentLibrary,
  listAgents,
} from '@/services/api';
import { chatURL } from '@/services/chat';

/** Agent 发现与个人列表使用真实数据；创建后直接进入个人列表。 */
export default function AgentsPage() {
  const { pathname } = useLocation();
  const mine = pathname === '/my-agents';
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

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 是保存成功或手动重试时触发重新查询的显式版本号。
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
    if (!user) {
      history.push('/login?redirect=/my-agents');
      return;
    }
    setCreating(true);
  };
  return (
    <div className="page-content agent-directory">
      <header className="page-heading">
        <div>
          <h1>{mine ? '我的 Agent' : '发现 Agent'}</h1>
        </div>
        <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
          新建 Agent
        </Button>
      </header>
      <div className="directory-toolbar">
        <Input
          aria-label="搜索 Agent"
          placeholder="搜索名称、介绍或标签"
          prefix={<SearchOutlined />}
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          allowClear
          className="agent-search"
        />
      </div>
      {error ? (
        <Alert
          type="error"
          showIcon
          title="加载 Agent 失败"
          description={error}
          action={
            <Button onClick={() => setRevision((value) => value + 1)}>
              重试
            </Button>
          }
        />
      ) : loading ? (
        <Skeleton active paragraph={{ rows: 5 }} />
      ) : visible.length ? (
        <div className="agent-grid">
          {visible.map((agent) => (
            <article key={agent.id} className="agent-card">
              <div className="agent-card-top">
                <Avatar
                  shape="square"
                  size={48}
                  src={agent.avatar || undefined}
                  icon={<RobotOutlined />}
                />
                <Tag>{agent.visibility === 'public' ? '公共' : '私有'}</Tag>
              </div>
              <h2>{agent.name}</h2>
              {agent.description && <p>{agent.description}</p>}
              <div className="agent-tags">
                {(agent.tags ?? []).map((tag) => (
                  <Tag key={tag}>{tag}</Tag>
                ))}
              </div>
              <div className="agent-card-bottom">
                {agent.status === 'offline' ? (
                  <span>已停用</span>
                ) : agent.visibility === 'public' && !added.has(agent.id) ? (
                  <Button
                    size="small"
                    loading={adding === agent.id}
                    disabled={Boolean(adding)}
                    onClick={async () => {
                      if (!user) {
                        history.push(
                          `/login?redirect=${encodeURIComponent(chatURL(agent.id))}`,
                        );
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
                    }}
                  >
                    添加并对话
                  </Button>
                ) : (
                  <Link to={chatURL(agent.id)}>
                    进入对话 <ArrowRightOutlined />
                  </Link>
                )}
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="directory-empty">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description={
              <h2>
                {query
                  ? '没有匹配的 Agent'
                  : mine
                    ? '还没有自己的 Agent'
                    : '暂无公共 Agent'}
              </h2>
            }
          >
            <Button onClick={query ? () => setQuery('') : openCreate}>
              {query ? '清除搜索' : '新建 Agent'}
            </Button>
          </Empty>
        </div>
      )}
      {creating && (
        <AgentEditor
          onCancel={() => setCreating(false)}
          onSaved={() => {
            setCreating(false);
            setRevision((value) => value + 1);
            history.push('/my-agents');
          }}
        />
      )}
    </div>
  );
}
