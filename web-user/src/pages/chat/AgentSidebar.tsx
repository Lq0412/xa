import { useEffect, useMemo, useState } from 'react';
import { AgentAvatar } from '@/components/AgentAvatar';
import { Icon } from '@/components/Icon';
import type { Agent } from '@/services/api';
import { errorMessage } from '@/services/api';
import { type Conversation, listMessages } from '@/services/chat';

type Props = {
  agents: Agent[];
  activeAgentID?: string;
  userID?: string;
  conversations: Conversation[];
  liveAgentID?: string;
  livePreview?: string;
  loading: boolean;
  error: string;
  onRetry: () => void;
  onCreate: () => void;
  onSelectAgent: (agent: Agent) => void;
};

function oneLine(value: string) {
  return value.replace(/\s+/g, ' ').trim();
}

/** 用每个会话最新的一条真实消息做列表预览。 */
function usePreviews(conversations: Conversation[]) {
  const [previews, setPreviews] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 只作为重新读取预览的触发器。
  useEffect(() => {
    if (!conversations.length) {
      setPreviews({});
      setError('');
      return;
    }
    const controller = new AbortController();
    Promise.all(
      conversations.map(async (item) => {
        const page = await listMessages(
          item.id,
          controller.signal,
          undefined,
          1,
        );
        const last = page.messages.at(-1);
        return [item.agent_id, oneLine(last?.content || '')] as const;
      }),
    )
      .then((entries) => {
        if (!controller.signal.aborted) {
          setPreviews(Object.fromEntries(entries));
          setError('');
        }
      })
      .catch((cause: unknown) => {
        if (controller.signal.aborted) return;
        setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [conversations, revision]);
  return {
    previews,
    error,
    retry: () => setRevision((value) => value + 1),
  };
}

function groupOf(agent: Agent, userID?: string) {
  if (userID && agent.owner_id === userID) return 'mine';
  if (agent.visibility === 'private') return 'mine';
  return 'added';
}

/** 只保留可对话 Agent；会话由 Agent 唯一对话承载，不在侧栏重复展开。 */
export function AgentSidebar({
  agents,
  activeAgentID,
  userID,
  conversations,
  liveAgentID,
  livePreview = '',
  loading,
  error,
  onRetry,
  onCreate,
  onSelectAgent,
}: Props) {
  const [search, setSearch] = useState('');
  const previews = usePreviews(conversations);
  const filtered = useMemo(() => {
    const keyword = search.trim().toLocaleLowerCase();
    return agents.filter(
      (agent) => !keyword || agent.name.toLocaleLowerCase().includes(keyword),
    );
  }, [agents, search]);
  const groups = [
    { id: 'mine', title: '我的 Agent' },
    { id: 'added', title: '已添加' },
  ] as const;
  const previewOf = (agent: Agent) => {
    if (agent.id === liveAgentID && livePreview) return livePreview;
    return previews.previews[agent.id] || '';
  };
  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex items-center gap-2">
        <label className="flex min-w-0 flex-1 items-center gap-2 rounded-full border border-zinc-200 bg-white px-3 py-2 text-zinc-400">
          <Icon name="search" size={16} />
          <input
            className="w-full border-0 bg-transparent p-0 text-sm text-zinc-900 outline-none"
            aria-label="搜索 Agent"
            placeholder="搜索"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
        <button
          type="button"
          className="flex size-9 shrink-0 items-center justify-center rounded-full border border-zinc-200 bg-white text-zinc-700 hover:bg-zinc-50"
          onClick={onCreate}
          aria-label="新建 Agent"
        >
          <Icon name="plus" size={18} />
        </button>
      </div>
      <nav className="min-h-0 flex-1 overflow-auto" aria-label="可对话 Agent">
        {loading ? (
          <div className="px-2 py-6 text-center text-xs text-zinc-400">
            正在读取
          </div>
        ) : error ? (
          <div className="px-2 py-6 text-center text-xs text-zinc-500">
            <p>{error}</p>
            <button type="button" className="mt-2 underline" onClick={onRetry}>
              重试
            </button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-2 py-6 text-center text-xs text-zinc-400">
            没有 Agent
          </div>
        ) : (
          groups.map((group) => {
            const items = filtered
              .filter((agent) => groupOf(agent, userID) === group.id)
              .sort((a, b) => Number(b.is_primary) - Number(a.is_primary));
            if (!items.length) return null;
            return (
              <div key={group.id} className="mb-3">
                <p className="px-2 py-2 text-xs text-zinc-400">{group.title}</p>
                <div className="flex flex-col gap-1">
                  {items.map((agent) => {
                    const active = agent.id === activeAgentID;
                    const preview = previewOf(agent);
                    return (
                      <button
                        key={agent.id}
                        type="button"
                        aria-label={agent.name}
                        className={`agent-sidebar-select flex w-full min-w-0 items-center gap-2 rounded-xl px-2 py-2 text-left ${active ? 'bg-white shadow-sm' : 'hover:bg-white/70'}`}
                        aria-current={active ? 'page' : undefined}
                        onClick={() => onSelectAgent(agent)}
                      >
                        <AgentAvatar size={32} />
                        <span className="min-w-0 flex-1">
                          <span className="flex min-w-0 items-center gap-1">
                            <span className="truncate text-sm font-medium text-zinc-900">
                              {agent.name}
                            </span>
                            {agent.is_primary && (
                              <span className="shrink-0 rounded bg-amber-100 px-1 text-[10px] leading-4 text-amber-800">
                                主
                              </span>
                            )}
                          </span>
                          <span className="block truncate text-xs text-zinc-500">
                            {preview || '还没有消息'}
                          </span>
                        </span>
                      </button>
                    );
                  })}
                </div>
              </div>
            );
          })
        )}
        {previews.error && (
          <p role="status" className="px-2 py-2 text-xs text-rose-700">
            {previews.error}
            <button
              type="button"
              className="ml-2 underline"
              onClick={previews.retry}
            >
              重试
            </button>
          </p>
        )}
      </nav>
    </div>
  );
}
