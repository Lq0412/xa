import { useEffect, useRef, useState } from 'react';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { type AgentVersion, ApiError, errorMessage } from '@/services/api';
import {
  deleteMemory,
  getMemoryMarkdown,
  listMemories,
  listMemoryEvents,
  type MemoryEdit,
  type MemoryItem,
  type MemoryPolicyResult,
  type MemoryView,
  readMemoryPolicy,
  saveMemoryPolicy,
  updateMemory,
} from '@/services/memory';

/** 回复结束后抽取是异步的，稍等再读一次列表。 */
const REFRESH_AFTER_TURN_MS = 3000;
const namespacePattern = /^[a-z][a-z0-9_.-]{0,63}$/;

export type MemoryCue = { agentId: string; seq: number };

function formatTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (item: number) => String(item).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function sourceLabel(source: string) {
  if (source === 'auto') return '自动';
  if (source === 'user_edited') return '手动编辑';
  return source;
}

function validateDraft(draft: MemoryEdit) {
  if (draft.content.trim() === '') return '请填写记忆内容';
  if ([...draft.content].length > 2000) return '记忆内容不能超过 2000 字';
  if (!namespacePattern.test(draft.namespace)) return '命名空间格式不正确';
  if (
    !Number.isInteger(draft.importance) ||
    draft.importance < 1 ||
    draft.importance > 10
  ) {
    return '重要度需要是 1 到 10 的整数';
  }
  return '';
}

/** 同一 namespace 内置顶在前；分组标题按 namespace 排序。 */
function groupByNamespace(items: MemoryItem[]) {
  const sorted = [...items].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    if (a.importance !== b.importance) return b.importance - a.importance;
    return b.updated_at.localeCompare(a.updated_at);
  });
  const groups = new Map<string, MemoryItem[]>();
  for (const item of sorted) {
    const list = groups.get(item.namespace);
    if (list) list.push(item);
    else groups.set(item.namespace, [item]);
  }
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b));
}

function Switch({
  checked,
  disabled,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={(event) => {
        event.stopPropagation();
        onChange(!checked);
      }}
      className={`relative h-5 w-9 shrink-0 rounded-full transition-colors disabled:opacity-40 ${checked ? 'bg-zinc-900' : 'bg-zinc-300'}`}
    >
      <span
        className={`absolute top-0.5 left-0.5 size-4 rounded-full bg-white transition-transform ${checked ? 'translate-x-4' : ''}`}
      />
    </button>
  );
}

function Alert({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div
      className="mb-3 flex items-center justify-between gap-3 rounded-xl bg-rose-50 px-3 py-2 text-sm text-rose-800"
      role="alert"
    >
      {message}
      {onRetry && (
        <button
          type="button"
          className="shrink-0 text-xs underline"
          onClick={onRetry}
        >
          重新读取
        </button>
      )}
    </div>
  );
}

function Toast({ message }: { message: string }) {
  if (!message) return null;
  return (
    <p
      role="status"
      className="absolute top-14 left-1/2 z-10 -translate-x-1/2 rounded-full bg-zinc-900 px-3 py-1.5 text-xs whitespace-nowrap text-white shadow-sm"
    >
      {message}
    </p>
  );
}

type MemoryController = ReturnType<typeof useAgentMemory>;

/** 共享层和当前 Agent 分开请求；对话结束后延迟刷新一次。 */
export function useAgentMemory({
  agentId,
  version,
  editable,
  cue,
  onPolicySaved,
  onConflict,
}: {
  agentId: string;
  version?: AgentVersion;
  editable: boolean;
  cue: MemoryCue;
  onPolicySaved: (result: MemoryPolicyResult) => void;
  onConflict: () => void;
}) {
  const [revision, setRevision] = useState(0);
  const [userItems, setUserItems] = useState<MemoryItem[]>([]);
  const [agentItems, setAgentItems] = useState<MemoryItem[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [policySaving, setPolicySaving] = useState(false);
  const [detailOpen, setDetailOpen] = useState(false);
  const [preview, setPreview] = useState(false);
  const [markdown, setMarkdown] = useState<MemoryView>();
  const [markdownLoading, setMarkdownLoading] = useState(false);
  const onPolicySavedRef = useRef(onPolicySaved);
  const onConflictRef = useRef(onConflict);
  const versionRef = useRef(version);
  onPolicySavedRef.current = onPolicySaved;
  onConflictRef.current = onConflict;
  versionRef.current = version;
  const policy = readMemoryPolicy(version?.configuration.memory_policy);

  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(''), 2600);
    return () => clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    if (cue.seq === 0 || cue.agentId !== agentId) return;
    const timer = setTimeout(() => {
      setRevision((value) => value + 1);
    }, REFRESH_AFTER_TURN_MS);
    return () => clearTimeout(timer);
  }, [agentId, cue.agentId, cue.seq]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: revision 在对话结束、编辑和重试后触发重新读取。
  useEffect(() => {
    const controller = new AbortController();
    setError('');
    Promise.all([
      listMemories({ scope: 'user' }, controller.signal),
      listMemories({ scope: 'agent', agent_id: agentId }, controller.signal),
    ])
      .then(([users, agents]) => {
        if (controller.signal.aborted) return;
        setUserItems(users);
        setAgentItems(agents);
        setLoaded(true);
      })
      .catch((cause) => {
        if (controller.signal.aborted) return;
        setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agentId, revision]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: 开关和 revision 变化后重新读取 memory.md。
  useEffect(() => {
    if (!preview) return;
    const controller = new AbortController();
    setMarkdown(undefined);
    setMarkdownLoading(true);
    getMemoryMarkdown(agentId, controller.signal)
      .then((view) => {
        if (!controller.signal.aborted) setMarkdown(view);
      })
      .catch((cause) => {
        if (controller.signal.aborted) return;
        setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setMarkdownLoading(false);
      });
    return () => controller.abort();
  }, [agentId, policy.enabled, preview, revision]);

  const refresh = () => setRevision((value) => value + 1);
  const reportConflict = () => {
    setNotice('该记忆已被更新，已刷新');
    setRevision((value) => value + 1);
  };
  const toggle = async () => {
    const current = versionRef.current;
    if (!current || !editable || policySaving) return;
    const enabled = readMemoryPolicy(
      current.configuration.memory_policy,
    ).enabled;
    setPolicySaving(true);
    setError('');
    try {
      const result = await saveMemoryPolicy(agentId, {
        expected_version_id: current.id,
        enabled: !enabled,
      });
      onPolicySavedRef.current(result);
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        onConflictRef.current();
        reportConflict();
        return;
      }
      setError(errorMessage(cause));
    } finally {
      setPolicySaving(false);
    }
  };

  return {
    userItems,
    agentItems,
    loaded,
    count: userItems.length + agentItems.length,
    error,
    notice,
    policy,
    policySaving,
    detailOpen,
    setDetailOpen,
    preview,
    setPreview,
    markdown,
    markdownLoading,
    editable,
    refresh,
    reportConflict,
    toggle,
  };
}

/** 模块页上的记忆卡片。开关不进入详情。 */
export function MemoryCard({
  memory,
  date,
}: {
  memory: MemoryController;
  date: string;
}) {
  return (
    <div
      className="col-span-2 flex min-h-36 flex-col rounded-[20px] p-3 text-left"
      style={{
        background: 'linear-gradient(160deg, #e4d4ff 0%, #f7f3ff 100%)',
      }}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-sm font-medium">记忆</span>
        <Switch
          checked={memory.policy.enabled}
          disabled={!memory.editable || memory.policySaving}
          label={memory.policy.enabled ? '关闭记忆' : '开启记忆'}
          onChange={() => void memory.toggle()}
        />
      </div>
      <button
        type="button"
        className="mt-1 flex flex-1 flex-col text-left"
        aria-label="查看记忆"
        onClick={() => memory.setDetailOpen(true)}
      >
        <span className="text-xs text-zinc-700">
          {memory.loaded ? `${memory.count} 条` : '…'}
        </span>
        <span className="mt-auto pt-6 text-[11px] text-zinc-700">{date}</span>
      </button>
    </div>
  );
}

function MemoryHistory({ id }: { id: string }) {
  const [events, setEvents] = useState<
    Awaited<ReturnType<typeof listMemoryEvents>> | undefined
  >();
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    listMemoryEvents(id, controller.signal)
      .then((result) => {
        if (!controller.signal.aborted) setEvents(result);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [id]);
  if (error) return <Alert message={error} />;
  if (!events) return <Skeleton className="mt-3 h-10" />;
  if (events.length === 0) {
    return <p className="mt-3 text-xs text-zinc-400">没有历史</p>;
  }
  return (
    <ol className="mt-3 space-y-2 border-l border-zinc-200 pl-3">
      {events.map((event) => (
        <li key={event.id}>
          <p className="text-xs font-medium text-zinc-800">{event.op}</p>
          <p className="text-[11px] text-zinc-500">
            <time dateTime={event.created_at}>
              {formatTime(event.created_at)}
            </time>
          </p>
          {event.reason.trim() && (
            <p className="text-xs text-zinc-600">{event.reason}</p>
          )}
        </li>
      ))}
    </ol>
  );
}

function MemoryItemView({
  item,
  enabled,
  onChanged,
  onConflict,
}: {
  item: MemoryItem;
  enabled: boolean;
  onChanged: () => void;
  onConflict: () => void;
}) {
  const [mode, setMode] = useState<'view' | 'edit' | 'delete'>('view');
  const [history, setHistory] = useState(false);
  const [draft, setDraft] = useState<MemoryEdit>({
    content: item.content,
    namespace: item.namespace,
    importance: item.importance,
    pinned: item.pinned,
  });
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState('');
  const save = async () => {
    const message = validateDraft(draft);
    if (message) {
      setFormError(message);
      return;
    }
    setSaving(true);
    setFormError('');
    try {
      await updateMemory(item.id, draft);
      setMode('view');
      onChanged();
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 409) {
        setMode('view');
        onConflict();
        return;
      }
      setFormError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  const remove = async () => {
    setSaving(true);
    setFormError('');
    try {
      await deleteMemory(item.id);
      onChanged();
    } catch (cause) {
      setFormError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  return (
    <article className="rounded-2xl border border-zinc-200/80 bg-white/80 p-3">
      {mode === 'edit' ? (
        <form
          className="space-y-2"
          onSubmit={(event) => {
            event.preventDefault();
            void save();
          }}
        >
          <label className="block text-xs text-zinc-500">
            内容
            <textarea
              className="mt-1 min-h-24 w-full resize-none rounded-xl border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-900"
              aria-label="记忆内容"
              maxLength={2000}
              value={draft.content}
              disabled={saving}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  content: event.target.value,
                }))
              }
            />
          </label>
          <label className="block text-xs text-zinc-500">
            命名空间
            <input
              className="mt-1 h-8 w-full rounded-lg border border-zinc-200 bg-white px-2 text-sm text-zinc-900"
              aria-label="命名空间"
              value={draft.namespace}
              disabled={saving}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  namespace: event.target.value,
                }))
              }
            />
          </label>
          <div className="text-xs text-zinc-500">
            重要度
            <Select
              value={String(draft.importance)}
              disabled={saving}
              onValueChange={(value) =>
                setDraft((current) => ({
                  ...current,
                  importance: Number(value),
                }))
              }
            >
              <SelectTrigger className="mt-1" aria-label="重要度">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Array.from({ length: 10 }, (_, index) =>
                  String(index + 1),
                ).map((value) => (
                  <SelectItem key={value} value={value}>
                    {value}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <label className="flex items-center gap-2 text-xs text-zinc-600">
            <input
              type="checkbox"
              checked={draft.pinned}
              disabled={saving}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  pinned: event.target.checked,
                }))
              }
            />
            置顶
          </label>
          {formError && (
            <p className="text-xs text-rose-700" role="alert">
              {formError}
            </p>
          )}
          <div className="flex gap-2">
            <Button size="sm" type="submit" disabled={saving}>
              {saving ? '保存中…' : '保存'}
            </Button>
            <Button
              size="sm"
              variant="quiet"
              disabled={saving}
              onClick={() => {
                setDraft({
                  content: item.content,
                  namespace: item.namespace,
                  importance: item.importance,
                  pinned: item.pinned,
                });
                setFormError('');
                setMode('view');
              }}
            >
              取消
            </Button>
          </div>
        </form>
      ) : (
        <>
          <div className="flex items-start gap-1.5">
            {item.pinned && (
              <>
                <Icon
                  name="pin"
                  size={14}
                  className="mt-1 shrink-0 text-zinc-500"
                />
                <span className="sr-only">已置顶</span>
              </>
            )}
            <p className="text-sm leading-6 whitespace-pre-wrap text-zinc-800">
              {item.content}
            </p>
          </div>
          <p className="mt-2 flex flex-wrap items-center gap-1.5 text-[11px] text-zinc-500">
            <span>重要度 {item.importance}</span>
            <span className="rounded-full bg-zinc-100 px-1.5 py-0.5 text-zinc-600">
              {sourceLabel(item.source)}
            </span>
            <time dateTime={item.updated_at}>
              {formatTime(item.updated_at)}
            </time>
          </p>
          {formError && (
            <p className="mt-2 text-xs text-rose-700" role="alert">
              {formError}
            </p>
          )}
          {mode === 'delete' ? (
            <div className="mt-2 flex items-center justify-between gap-2 text-xs">
              <span className="text-zinc-700">删除这条记忆？</span>
              <span className="flex gap-3">
                <button
                  type="button"
                  className="text-rose-700 underline underline-offset-4"
                  disabled={saving}
                  onClick={() => void remove()}
                >
                  删除
                </button>
                <button
                  type="button"
                  className="text-zinc-500 underline underline-offset-4"
                  disabled={saving}
                  onClick={() => setMode('view')}
                >
                  取消
                </button>
              </span>
            </div>
          ) : (
            <div className="mt-2 flex gap-3 text-xs">
              <button
                type="button"
                className="text-zinc-800 underline underline-offset-4 disabled:opacity-40"
                disabled={!enabled}
                onClick={() => {
                  setFormError('');
                  setMode('edit');
                }}
              >
                编辑
              </button>
              <button
                type="button"
                className="text-zinc-800 underline underline-offset-4 disabled:opacity-40"
                disabled={!enabled}
                onClick={() => {
                  setFormError('');
                  setMode('delete');
                }}
              >
                删除
              </button>
              <button
                type="button"
                className="text-zinc-800 underline underline-offset-4"
                aria-expanded={history}
                onClick={() => setHistory((value) => !value)}
              >
                历史
              </button>
            </div>
          )}
          {history && <MemoryHistory id={item.id} />}
        </>
      )}
    </article>
  );
}

function MemoryGroup({
  title,
  items,
  enabled,
  onChanged,
  onConflict,
}: {
  title: string;
  items: MemoryItem[];
  enabled: boolean;
  onChanged: () => void;
  onConflict: () => void;
}) {
  const groups = groupByNamespace(items);
  return (
    <section className="mt-5 first:mt-0">
      <h3 className="text-sm font-medium">{title}</h3>
      {items.length === 0 ? (
        <p className="mt-2 text-xs text-zinc-400">暂无</p>
      ) : (
        groups.map(([namespace, grouped]) => (
          <div key={namespace} className="mt-3">
            <h4 className="mb-2 text-[11px] font-medium text-zinc-400">
              {namespace}
            </h4>
            <div className="space-y-2">
              {grouped.map((item) => (
                <MemoryItemView
                  key={item.id}
                  item={item}
                  enabled={enabled}
                  onChanged={onChanged}
                  onConflict={onConflict}
                />
              ))}
            </div>
          </div>
        ))
      )}
    </section>
  );
}

/** 记忆详情占满整个配置栏，编辑和删除都留在这一层。 */
export function MemoryDetail({
  memory,
  onBack,
}: {
  memory: MemoryController;
  onBack: () => void;
}) {
  const enabled = memory.policy.enabled;
  const showEmpty = memory.loaded && memory.count === 0;
  return (
    <section
      className="relative flex h-full min-h-0 flex-col bg-white"
      aria-label="Agent 配置"
    >
      <Toast message={memory.notice} />
      <header className="flex shrink-0 items-center gap-2 px-4 py-3">
        <button
          type="button"
          className="inline-flex items-center gap-1 text-sm text-zinc-600"
          onClick={onBack}
        >
          <Icon name="arrow-left" size={16} />
          返回
        </button>
        <h2 className="text-sm font-medium">记忆</h2>
        <div className="ml-auto flex items-center gap-2 text-xs text-zinc-600">
          <span>memory.md 预览</span>
          <Switch
            checked={memory.preview}
            label="memory.md 预览"
            onChange={memory.setPreview}
          />
        </div>
      </header>
      <div className="min-h-0 flex-1 overflow-auto px-4 pb-6">
        {memory.error && (
          <Alert message={memory.error} onRetry={memory.refresh} />
        )}
        {!enabled && (
          <p className="mb-4 rounded-xl bg-zinc-100 px-3 py-2 text-xs leading-5 text-zinc-600">
            记忆已关闭，这些内容不会参与后续对话。
          </p>
        )}
        {memory.preview ? (
          <div>
            {memory.markdownLoading && !memory.markdown ? (
              <Skeleton className="h-28" />
            ) : (
              memory.markdown && (
                <>
                  <p className="mb-3 text-xs text-zinc-500">
                    约 {memory.markdown.estimated_tokens} tokens
                  </p>
                  {memory.markdown.markdown ? (
                    <div className="text-sm text-zinc-800">
                      <Markdown>{memory.markdown.markdown}</Markdown>
                    </div>
                  ) : (
                    enabled &&
                    showEmpty && (
                      <p className="text-sm leading-6 text-zinc-500">
                        还没有记忆，多聊几句后会自动整理。
                      </p>
                    )
                  )}
                </>
              )
            )}
          </div>
        ) : (
          <div className={enabled ? undefined : 'opacity-40 grayscale'}>
            {!memory.loaded && !memory.error && <Skeleton className="h-24" />}
            {showEmpty && (
              <p className="py-6 text-center text-sm leading-6 text-zinc-500">
                还没有记忆，多聊几句后会自动整理。
              </p>
            )}
            {memory.loaded && memory.count > 0 && (
              <>
                <MemoryGroup
                  title="关于你（所有 Agent 共享）"
                  items={memory.userItems}
                  enabled={enabled}
                  onChanged={memory.refresh}
                  onConflict={memory.reportConflict}
                />
                <MemoryGroup
                  title="这个 Agent 的记忆"
                  items={memory.agentItems}
                  enabled={enabled}
                  onChanged={memory.refresh}
                  onConflict={memory.reportConflict}
                />
              </>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
