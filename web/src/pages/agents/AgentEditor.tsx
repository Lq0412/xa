import { ArrowLeft, ChevronRight } from 'lucide-react';
import {
  type FormEvent,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import { history, Link, useParams } from 'umi';
import { useAdminAccess } from '@/components/AdminLayout';
import { Badge } from '@/components/DataTable';
import { ErrorNotice } from '@/components/Page';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input, Select, Textarea } from '@/components/ui/fields';
import { useResource } from '@/lib/useResource';
import Loading from '@/loading';
import {
  type AgentSaveInput,
  type CatalogTool,
  createAgent,
  deleteAgent,
  getAgent,
  getAgentConfig,
  listTools,
  saveAgent,
  setAgentStatus,
} from '@/services/xa/api';
import { ApiError } from '@/services/xa/request';

type EditorData = {
  agent?: XA.Agent;
  version?: XA.AgentVersion;
  tools: CatalogTool[];
};

/** 配置路由先获取完整版本与工具目录，刷新后仍可直接编辑。 */
export default function AgentEditorPage() {
  const { id } = useParams<{ id: string }>();
  const load = useCallback(async (): Promise<EditorData> => {
    const [agent, version, catalog] = await Promise.all([
      id ? getAgent(id) : undefined,
      id ? getAgentConfig(id) : undefined,
      listTools(),
    ]);
    return { agent, version, tools: catalog.tools };
  }, [id]);
  const { data, loading, error, reload } = useResource(load);
  if (loading) return <Loading />;
  if (error || !data)
    return (
      <div className="page-content">
        <ErrorNotice>{error}</ErrorNotice>
        <Button variant="ghost" onClick={reload}>
          重试
        </Button>
      </div>
    );
  return (
    <AgentEditor
      key={`${id || 'new'}-${data.version?.id}`}
      data={data}
      onSaved={reload}
    />
  );
}

/** 整页编辑保留所有未展示字段；工具审批与 SOUL 一起写入新绑定版本。 */
function AgentEditor({
  data: { agent, version, tools },
  onSaved,
}: {
  data: EditorData;
  onSaved: () => void;
}) {
  const { canManageAgent } = useAdminAccess();
  const readonly = !!agent && !canManageAgent(agent);
  const initial: AgentSaveInput = {
    name: agent?.name ?? '',
    description: agent?.description ?? '',
    avatar: agent?.avatar ?? '',
    tags: agent?.tags ?? [],
    visibility: agent?.visibility ?? 'public',
    configuration: version?.configuration ?? { soul_md: '', model: '' },
  };
  const [draft, setDraft] = useState(initial);
  const [tags, setTags] = useState(initial.tags.join(', '));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [deleting, setDeleting] = useState(false);
  const [saved, setSaved] = useState(false);
  const releaseNavigation = useRef<(() => void) | undefined>(undefined);
  const dirty =
    !saved &&
    (JSON.stringify(draft) !== JSON.stringify(initial) ||
      tags !== initial.tags.join(', '));
  // 浏览器刷新和应用内离开都保护未保存内容。
  useEffect(() => {
    if (!dirty) return;
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener('beforeunload', beforeUnload);
    const unblock = history.block((transition) => {
      if (window.confirm('放弃未保存的更改？')) {
        unblock();
        transition.retry();
      }
    });
    releaseNavigation.current = unblock;
    return () => {
      window.removeEventListener('beforeunload', beforeUnload);
      unblock();
      releaseNavigation.current = undefined;
    };
  }, [dirty]);

  function configuration(patch: Partial<XA.Configuration>) {
    setDraft((value) => ({
      ...value,
      configuration: { ...value.configuration, ...patch },
    }));
  }
  function updateTool(tool: CatalogTool, patch: Partial<XA.ToolBinding>) {
    const bindings = [...(draft.configuration.tools ?? [])];
    const index = bindings.findIndex((binding) => binding.name === tool.name);
    const binding =
      index >= 0
        ? bindings[index]
        : {
            name: tool.name,
            enabled: tool.default_enabled,
            approval: tool.approval.default,
          };
    if (index >= 0) bindings[index] = { ...binding, ...patch };
    else bindings.push({ ...binding, ...patch });
    configuration({ tools: bindings });
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (readonly || saving) return;
    setSaving(true);
    setError('');
    const input = {
      ...draft,
      name: draft.name.trim(),
      tags: tags
        .split(/[,，]/)
        .map((tag) => tag.trim())
        .filter(Boolean),
      configuration: { ...draft.configuration, prompts: undefined },
    };
    if (!input.name) {
      setError('请输入 Agent 名称');
      setSaving(false);
      return;
    }
    try {
      if (agent && version) {
        await saveAgent(agent.id, version.id, input);
        setSaved(true);
        onSaved();
      } else {
        const result = await createAgent(input);
        setSaved(true);
        releaseNavigation.current?.();
        history.replace(`/agents/${encodeURIComponent(result.agent.id)}`);
      }
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      setError(
        cause.status === 409
          ? '配置已被更新，请重新加载后核对并保存。'
          : cause.message,
      );
    } finally {
      setSaving(false);
    }
  }
  async function changeStatus(remove = false) {
    if (!agent || readonly || dirty) return;
    setSaving(true);
    setError('');
    try {
      if (remove) {
        await deleteAgent(agent.id);
        history.replace('/agents');
      } else {
        await setAgentStatus(agent.id, agent.status !== 'active');
        onSaved();
      }
    } catch (cause) {
      if (!(cause instanceof ApiError)) throw cause;
      setError(cause.message);
    } finally {
      setSaving(false);
      setDeleting(false);
    }
  }
  const modelOptions = version?.available_models ?? [];
  const memoryEnabled = draft.configuration.memory_policy?.enabled !== false;
  return (
    <div className="page-content config-page">
      <Link
        to="/agents"
        className="mb-7 inline-flex items-center gap-2 text-xs text-zinc-500"
      >
        <ArrowLeft size={14} />
        Agent
      </Link>
      <div className="page-heading">
        <div className="flex items-center gap-3">
          <img
            className="size-10 rounded-xl"
            alt=""
            src={agent?.avatar || '/default-agent-avatar.png'}
          />
          <h1>{agent?.name || '新建 Agent'}</h1>
        </div>
        {agent && (
          <Badge tone={agent.status === 'active' ? 'green' : 'neutral'}>
            {agent.status === 'active' ? '已启用' : '已停用'}
          </Badge>
        )}
      </div>
      <form id="agent-config" onSubmit={submit}>
        {error && <ErrorNotice>{error}</ErrorNotice>}
        <fieldset disabled={readonly || saving}>
          <section className="config-section pt-0">
            <h2>基本信息</h2>
            <div className="config-grid">
              <label className="field-label" htmlFor="agent-name">
                名称
                <Input
                  id="agent-name"
                  value={draft.name}
                  required
                  maxLength={120}
                  onChange={(event) =>
                    setDraft({ ...draft, name: event.target.value })
                  }
                />
              </label>
              <label className="field-label" htmlFor="agent-visibility">
                可见范围
                <Select
                  id="agent-visibility"
                  value={draft.visibility}
                  disabled={!!agent}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      visibility: event.target.value as XA.Agent['visibility'],
                    })
                  }
                >
                  <option value="public">公共</option>
                  <option value="private">我的</option>
                </Select>
              </label>
              <label
                className="field-label sm:col-span-2"
                htmlFor="agent-description"
              >
                简介
                <Input
                  id="agent-description"
                  maxLength={2000}
                  value={draft.description}
                  onChange={(event) =>
                    setDraft({ ...draft, description: event.target.value })
                  }
                />
              </label>
              <label className="field-label sm:col-span-2" htmlFor="agent-tags">
                标签
                <Input
                  id="agent-tags"
                  value={tags}
                  placeholder="用逗号分隔"
                  onChange={(event) => setTags(event.target.value)}
                />
              </label>
            </div>
          </section>
          <section className="config-section">
            <h2>
              <label htmlFor="agent-soul">SOUL.md</label>
            </h2>
            <Textarea
              id="agent-soul"
              rows={10}
              maxLength={20000}
              className="text-[13px] leading-7"
              value={draft.configuration.soul_md ?? ''}
              onChange={(event) =>
                configuration({ soul_md: event.target.value })
              }
            />
          </section>
          <section className="config-section">
            <h2>
              <label htmlFor="agent-model">模型</label>
            </h2>
            <Select
              id="agent-model"
              className="max-w-sm"
              value={draft.configuration.model ?? ''}
              disabled={!version}
              onChange={(event) => configuration({ model: event.target.value })}
            >
              {!version && <option value="">平台默认模型</option>}
              {version &&
                draft.configuration.model &&
                !modelOptions.includes(draft.configuration.model) && (
                  <option value={draft.configuration.model}>
                    {draft.configuration.model}（当前）
                  </option>
                )}
              {modelOptions.map((model) => (
                <option key={model}>{model}</option>
              ))}
            </Select>
          </section>
          <section className="config-section">
            <h2>工具</h2>
            {tools.map((tool) => {
              const binding = draft.configuration.tools?.find(
                (item) => item.name === tool.name,
              );
              const enabled = binding?.enabled ?? tool.default_enabled;
              const approval =
                !tool.approval.overridable && tool.approval.default === 'always'
                  ? 'always'
                  : binding?.approval || tool.approval.default;
              return (
                <div className="setting-row" key={tool.name}>
                  <div className="flex items-center gap-3">
                    <button
                      type="button"
                      role="switch"
                      aria-checked={enabled && tool.available}
                      aria-label={`启用${tool.title}`}
                      className="tool-switch"
                      disabled={!tool.available}
                      onClick={() => updateTool(tool, { enabled: !enabled })}
                    >
                      <span />
                    </button>
                    <span>{tool.title}</span>
                  </div>
                  {!tool.available ? (
                    <span className="text-xs text-zinc-400">未开放</span>
                  ) : (
                    <button
                      type="button"
                      disabled={!enabled || !tool.approval.overridable}
                      aria-label={`${tool.title}：${approval === 'always' ? '需确认' : '自动运行'}`}
                      className="text-xs text-zinc-500 enabled:hover:text-zinc-900 disabled:opacity-50"
                      onClick={() =>
                        updateTool(tool, {
                          approval: approval === 'always' ? 'never' : 'always',
                        })
                      }
                    >
                      {approval === 'always' ? '需确认' : '自动运行'}
                    </button>
                  )}
                </div>
              );
            })}
            {!memoryEnabled && (
              <p className="mt-4 text-xs text-zinc-500">
                记忆已关闭，记忆工具不会执行。
              </p>
            )}
          </section>
        </fieldset>
        <details className="config-disclosure">
          <summary>
            <span>记忆</span>
            <span className="ml-auto text-xs text-zinc-500">
              {memoryEnabled ? '已开启' : '已关闭'}
            </span>
            <ChevronRight size={14} />
          </summary>
          <PolicyFields
            values={draft.configuration.memory_policy}
            labels={{
              enabled: '启用',
              allow_forget: '允许遗忘',
              recall_top_k: '召回条数',
              decay_enabled: '记忆衰减',
              auto_extract: '自动提取',
              recall_token_budget: '召回预算',
            }}
          />
        </details>
        <details className="config-disclosure">
          <summary>
            <span>运行限制</span>
            <span className="ml-auto text-xs text-zinc-500">
              {draft.configuration.runtime_limits ? '当前配置' : '平台默认'}
            </span>
            <ChevronRight size={14} />
          </summary>
          <PolicyFields
            values={draft.configuration.runtime_limits}
            labels={{
              max_model_turns: '模型轮数',
              max_tool_calls: '工具调用次数',
              max_parallel_tools: '并行工具数',
              max_calls_per_step: '每步调用次数',
              max_wall_time_ms: '最长运行时间（毫秒）',
              max_total_tokens: '总 Token 上限',
              max_output_tokens: '输出 Token 上限',
              tool_result_max_tokens: '工具结果 Token 上限',
              history_tool_runs: '历史工具轮数',
            }}
          />
        </details>
      </form>
      {agent && !readonly && (
        <section className="config-section border-0">
          <h2>危险区</h2>
          <div className="setting-row">
            <span>
              {agent.status === 'active' ? '停用 Agent' : '启用 Agent'}
            </span>
            <Button
              variant="ghost"
              size="sm"
              disabled={saving || dirty}
              onClick={() => void changeStatus()}
            >
              {agent.status === 'active' ? '停用' : '启用'}
            </Button>
          </div>
          {!agent.is_primary && (
            <div className="setting-row">
              <span>删除 Agent</span>
              <Button
                variant="ghost"
                size="sm"
                className="text-red-700"
                disabled={saving || dirty}
                onClick={() => setDeleting(true)}
              >
                删除
              </Button>
            </div>
          )}
        </section>
      )}
      {!readonly && (dirty || !agent) && (
        <div className="save-bar">
          <span className="text-xs text-zinc-500">未保存的更改</span>
          <div className="flex gap-2">
            <Button
              variant="ghost"
              disabled={saving}
              onClick={() => {
                setDraft(initial);
                setTags(initial.tags.join(', '));
              }}
            >
              取消
            </Button>
            <Button form="agent-config" type="submit" disabled={saving}>
              {saving ? '保存中…' : agent ? '保存' : '创建'}
            </Button>
          </div>
        </div>
      )}
      <Dialog
        open={deleting}
        onOpenChange={(open) => {
          if (!saving) setDeleting(open);
        }}
      >
        <DialogContent className="top-1/2 left-1/2 w-[calc(100%-3rem)] max-w-sm -translate-x-1/2 -translate-y-1/2 space-y-5 rounded-xl p-6">
          <DialogTitle className="text-lg font-semibold">
            删除「{agent?.name}」？
          </DialogTitle>
          <DialogDescription className="text-sm text-zinc-500">
            删除后将从 Agent 列表移除。
          </DialogDescription>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              disabled={saving}
              onClick={() => setDeleting(false)}
            >
              取消
            </Button>
            <Button
              className="bg-red-700 text-white hover:bg-red-800"
              disabled={saving}
              onClick={() => void changeStatus(true)}
            >
              删除
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

/** 仅展示已返回的策略字段；缺省配置交给服务端，不猜测部署默认值。 */
function PolicyFields({
  values,
  labels,
}: {
  values?: Record<string, unknown> | null;
  labels: Record<string, string>;
}) {
  const entries = Object.entries(values ?? {}).filter(([key]) => key in labels);
  return entries.length ? (
    <dl className="detail-fields mt-0">
      {entries.map(([key, value]) => (
        <div key={key} className="contents">
          <dt>{labels[key]}</dt>
          <dd>
            {typeof value === 'boolean'
              ? value
                ? '开启'
                : '关闭'
              : String(value)}
          </dd>
        </div>
      ))}
    </dl>
  ) : (
    <p className="pb-6 text-xs text-zinc-500">使用平台默认值</p>
  );
}
