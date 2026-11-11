import { useEffect, useRef, useState } from 'react';
import { type Agent, getAgent } from '@/services/api';
import { ApiError, errorMessage } from '@/services/http';
import {
  type AgentTool,
  type AgentToolSettings,
  getAgentTools,
  getToolCatalog,
  saveAgentTools,
} from '@/services/tools';
import { RuntimeLimitsDialog, ToolConfigDialog } from './ToolConfigDialog';

const textButton = 'shrink-0 py-1 text-xs hover:underline disabled:opacity-45';
const conflictMessage = '配置已被更新，请刷新';

function unavailableReason(tool: AgentTool) {
  if (!tool.available) return '即将上线';
  if (tool.reason === 'memory_disabled') return '需开启记忆';
  if (tool.reason === 'role_denied') return '无使用权限';
  return undefined;
}

/** 编辑界面固定打开时的版本，刷新列表时也保留该版本用于冲突检查。 */
type ToolEditor = {
  versionId: string;
  target:
    | { tool: AgentTool; hasTimezone: boolean; description?: string }
    | { limits: AgentToolSettings['runtime_limits'] };
};

/** 仅归属用户可查看和保存工具配置。 */
export function AgentTools({
  agent,
  onSaved,
}: {
  agent: Agent;
  onSaved: (agent: Agent) => void;
}) {
  const [settings, setSettings] = useState<AgentToolSettings>();
  const [catalog, setCatalog] = useState<
    Awaited<ReturnType<typeof getToolCatalog>>['tools']
  >([]);
  const [editor, setEditor] = useState<ToolEditor>();
  const [error, setError] = useState('');
  const [editError, setEditError] = useState('');
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const savingRef = useRef(false);
  const mounted = useRef(false);
  const refresh = () => setRevision((value) => value + 1);

  useEffect(() => {
    mounted.current = true;
    const changed = () => setRevision((value) => value + 1);
    window.addEventListener('xa:tools-updated', changed);
    return () => {
      mounted.current = false;
      window.removeEventListener('xa:tools-updated', changed);
    };
  }, []);
  // biome-ignore lint/correctness/useExhaustiveDependencies: 版本和刷新请求都需要重新读取生效配置。
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    Promise.all([
      getAgentTools(agent.id, controller.signal),
      getToolCatalog(controller.signal),
    ])
      .then(([tools, specs]) => {
        if (controller.signal.aborted) return;
        setSettings(tools);
        setCatalog(specs.tools);
      })
      .catch((cause) => {
        if (controller.signal.aborted) return;
        setSettings(undefined);
        setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [agent.id, agent.current_version_id, revision]);

  const save = async (
    versionId: string,
    edit: Parameters<typeof saveAgentTools>[2],
    inDialog = false,
  ) => {
    if (savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    setError('');
    setEditError('');
    try {
      await saveAgentTools(agent.id, versionId, edit);
      // AgentDetails 按 Agent ID 重新挂载，已经离开的实例不再更新界面。
      if (!mounted.current) return;
      const updated = await getAgent(agent.id);
      if (!mounted.current) return;
      onSaved(updated);
      setEditor(undefined);
      setLoading(true);
      refresh();
    } catch (cause) {
      if (!mounted.current) return;
      if (cause instanceof ApiError && cause.status === 409) {
        setEditor(undefined);
        setError(conflictMessage);
      } else {
        (inDialog ? setEditError : setError)(errorMessage(cause));
      }
    } finally {
      savingRef.current = false;
      if (mounted.current) setSaving(false);
    }
  };
  const closeEditor = () => {
    setEditor(undefined);
    setEditError('');
  };
  const editDisabled = saving || loading || error === conflictMessage;

  return (
    <section className="mt-[18px] text-left" aria-label="工具设置">
      <div className="mb-2 flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">工具</h3>
        {settings && (
          <button
            type="button"
            className={`${textButton} text-zinc-600`}
            disabled={editDisabled}
            onClick={() =>
              setEditor({
                versionId: settings.current_version_id,
                target: { limits: settings.runtime_limits },
              })
            }
          >
            运行限制
          </button>
        )}
      </div>
      {settings?.tools.map((tool) => {
        const spec = catalog.find((item) => item.name === tool.name);
        const reason = unavailableReason(tool);
        const status = tool.approval === 'always' ? '需确认' : '自动运行';
        const approvalStyle =
          tool.approval === 'always' ? 'font-medium' : 'text-zinc-500';
        return (
          <div key={tool.name} className="py-1.5">
            <div className="flex min-h-7 flex-wrap items-center justify-between gap-x-3 text-[13px]">
              <span>{tool.title}</span>
              <div className="ml-auto flex items-center gap-3">
                {tool.enabled &&
                  (tool.approval_overridable ? (
                    <button
                      type="button"
                      className={`${textButton} ${approvalStyle}`}
                      aria-label={`${tool.title}：${status}`}
                      disabled={editDisabled}
                      onClick={() =>
                        void save(settings.current_version_id, {
                          tool,
                          changes: {
                            approval:
                              tool.approval === 'always' ? 'never' : 'always',
                          },
                        })
                      }
                    >
                      {status}
                    </button>
                  ) : (
                    <span className={`text-xs ${approvalStyle}`}>{status}</span>
                  ))}
                <button
                  type="button"
                  className={`${textButton} ${tool.enabled ? 'font-medium' : 'text-zinc-500'}`}
                  aria-label={`启用${tool.title}`}
                  aria-pressed={tool.enabled}
                  disabled={editDisabled || Boolean(reason)}
                  onClick={() =>
                    void save(settings.current_version_id, {
                      tool,
                      changes: { enabled: !tool.enabled },
                    })
                  }
                >
                  {reason ?? (tool.enabled ? '已启用' : '已停用')}
                </button>
                {!reason && (
                  <button
                    type="button"
                    className={`${textButton} text-zinc-600`}
                    aria-label={`配置${tool.title}`}
                    disabled={editDisabled}
                    onClick={() =>
                      setEditor({
                        versionId: settings.current_version_id,
                        target: {
                          tool,
                          description: spec?.description,
                          hasTimezone:
                            spec?.config_schema.properties?.timezone?.type ===
                            'string',
                        },
                      })
                    }
                  >
                    配置
                  </button>
                )}
              </div>
            </div>
          </div>
        );
      })}
      {error && (
        <div className="mt-1 flex items-center justify-between gap-3 text-xs text-rose-600">
          <p role="alert">{error}</p>
          <button
            type="button"
            className={textButton}
            disabled={saving || loading}
            onClick={refresh}
          >
            刷新
          </button>
        </div>
      )}
      {editor &&
        ('tool' in editor.target ? (
          <ToolConfigDialog
            tool={editor.target.tool}
            description={editor.target.description}
            hasTimezone={editor.target.hasTimezone}
            error={editError}
            busy={saving}
            onClose={closeEditor}
            onSave={(changes) => {
              if (!('tool' in editor.target)) return;
              if (Object.keys(changes).length === 0) closeEditor();
              else
                void save(
                  editor.versionId,
                  { tool: editor.target.tool, changes },
                  true,
                );
            }}
          />
        ) : (
          <RuntimeLimitsDialog
            limits={editor.target.limits}
            error={editError}
            busy={saving}
            onClose={closeEditor}
            onSave={(runtimeLimits) =>
              void save(editor.versionId, { runtimeLimits }, true)
            }
          />
        ))}
    </section>
  );
}
