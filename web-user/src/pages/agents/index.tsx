import { history, Link, useLocation, useModel } from '@umijs/max';
import { useEffect, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { AgentEditor } from '@/components/AgentEditor';
import { Icon } from '@/components/Icon';
import { Button } from '@/components/ui/button';
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
    <div className="mx-auto w-full max-w-6xl px-8 py-10">
      <header className="mb-6 flex items-center justify-between gap-4">
        <h1 className="text-2xl font-medium">
          {mine ? '我的 Agent' : '发现 Agent'}
        </h1>
        <Button onClick={openCreate}>
          <Icon name="plus" size={16} />
          新建 Agent
        </Button>
      </header>
      <label className="mb-6 flex w-full max-w-xs items-center gap-2 rounded-full border border-zinc-200 bg-white px-3 py-2 text-zinc-400">
        <Icon name="search" size={16} />
        <input
          className="w-full border-0 bg-transparent p-0 text-sm text-zinc-900 outline-none"
          aria-label="搜索 Agent"
          placeholder="搜索"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
      </label>
      {error ? (
        <div
          role="alert"
          className="flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3.5 py-3 text-sm text-rose-800"
        >
          {error}
          <button
            type="button"
            className="shrink-0 underline"
            onClick={() => setRevision((value) => value + 1)}
          >
            重试
          </button>
        </div>
      ) : loading ? (
        <p className="text-sm text-zinc-500" role="status">
          正在读取 Agent…
        </p>
      ) : visible.length ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-4">
          {visible.map((agent) => (
            <article
              key={agent.id}
              className="flex min-w-0 flex-col rounded-2xl border border-zinc-200 bg-white p-5"
            >
              <div className="mb-4 flex items-center justify-between">
                <AgentAvatar size={40} />
                <span className="text-xs text-zinc-400">
                  {agent.is_primary
                    ? '主 Agent'
                    : agent.visibility === 'public'
                      ? '公共'
                      : '私有'}
                </span>
              </div>
              <h2 className="text-base font-medium">{agent.name}</h2>
              {agent.description && (
                <p className="mt-2 line-clamp-3 text-sm leading-6 text-zinc-500">
                  {agent.description}
                </p>
              )}
              <div className="mt-3 flex flex-wrap gap-1 text-xs text-zinc-400">
                {(agent.tags ?? []).map((tag) => (
                  <span key={tag}>{tag}</span>
                ))}
              </div>
              <div className="mt-auto pt-5 text-sm">
                {agent.status === 'offline' ? (
                  <span className="text-zinc-400">已停用</span>
                ) : agent.visibility === 'public' && !added.has(agent.id) ? (
                  <Button
                    variant="quiet"
                    disabled={Boolean(adding)}
                    onClick={() => void add(agent)}
                  >
                    {adding === agent.id ? '添加中…' : '添加并对话'}
                  </Button>
                ) : (
                  <Link
                    className="inline-flex items-center gap-1"
                    to={chatURL(agent.id)}
                  >
                    进入对话 <Icon name="arrow-right" size={16} />
                  </Link>
                )}
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="py-16 text-zinc-500">
          <h2 className="mb-4 text-lg font-medium text-zinc-800">
            {query
              ? '没有匹配的 Agent'
              : mine
                ? '还没有自己的 Agent'
                : '暂无公共 Agent'}
          </h2>
          <Button
            variant="outline"
            onClick={query ? () => setQuery('') : openCreate}
          >
            {query ? '清除搜索' : '新建 Agent'}
          </Button>
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
