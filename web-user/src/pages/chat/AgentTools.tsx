import { type ReactNode, useEffect, useEffectEvent, useState } from 'react';
import { Icon } from '@/components/Icon';
import type { Agent } from '@/services/api';
import { errorMessage } from '@/services/http';
import {
  type AgentToolSettings,
  getAgentTools,
  saveAgentTools,
} from '@/services/tools';
import { Switch } from './MemoryPanel';
import { RuntimeLimitsDialog, ToolConfigDialog } from './ToolConfigDialog';

/** 设置页只提供两项写操作的确认开关。其余配置放在高级。 */
export function AgentTools({
  agent,
  onSaved,
  editable,
  advanced,
}: {
  agent: Agent;
  onSaved: (agent: Agent) => void;
  editable: boolean;
  advanced: ReactNode;
}) {
  const [settings, setSettings] = useState<AgentToolSettings>();
  const [dialog, setDialog] = useState<'limits' | 'time'>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  const applySettings = useEffectEvent((value: AgentToolSettings) => {
    setSettings(value);
    if (value.current_version_id !== agent.current_version_id) {
      onSaved({ ...agent, current_version_id: value.current_version_id });
    }
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: Agent 版本变化后读取生效配置。
  useEffect(() => {
    if (!editable) return;
    const controller = new AbortController();
    const load = () => {
      setError('');
      getAgentTools(agent.id, controller.signal)
        .then((value) => {
          if (!controller.signal.aborted) applySettings(value);
        })
        .catch((cause) => {
          if (!controller.signal.aborted) setError(errorMessage(cause));
        });
    };
    load();
    window.addEventListener('xa:tools-updated', load);
    return () => {
      controller.abort();
      window.removeEventListener('xa:tools-updated', load);
    };
  }, [agent.id, agent.current_version_id, editable, revision]);

  const save = async (edit: Parameters<typeof saveAgentTools>[2]) => {
    if (!settings || busy) return;
    setBusy(true);
    setError('');
    try {
      const result = await saveAgentTools(
        agent.id,
        settings.current_version_id,
        edit,
      );
      setSettings(result);
      setDialog(undefined);
      onSaved({ ...agent, current_version_id: result.current_version_id });
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  };
  const stale = settings?.current_version_id !== agent.current_version_id;
  const time = settings?.tools.find(
    (tool) => tool.name === 'current_time' && tool.available,
  );

  return (
    <>
      {editable && (
        <div className="mt-4 divide-y divide-zinc-100">
          {['memory_forget', 'memory_write'].map((name) => {
            const tool = settings?.tools.find((item) => item.name === name);
            if (!tool?.available || !tool.approval_overridable) return null;
            const checked = tool.approval === 'always';
            return (
              <div
                key={name}
                className="flex min-h-12 items-center justify-between"
              >
                <span>{tool.title}</span>
                <Switch
                  checked={checked}
                  disabled={busy || stale}
                  label={`${tool.title}：${checked ? '先问我' : '直接执行'}`}
                  onChange={(enabled) =>
                    void save({
                      tool,
                      changes: { approval: enabled ? 'always' : 'never' },
                    })
                  }
                />
              </div>
            );
          })}
        </div>
      )}
      {!dialog && error && (
        <p role="alert" className="my-2 text-xs text-rose-700">
          {error}{' '}
          <button
            type="button"
            className="underline"
            onClick={() => setRevision((value) => value + 1)}
          >
            重新读取
          </button>
        </p>
      )}
      <details className="mt-6 border-t border-zinc-200 pt-2">
        <summary className="flex min-h-10 cursor-pointer list-none items-center justify-between text-zinc-500">
          高级 <Icon name="chevron" size={14} className="rotate-90" />
        </summary>
        {advanced}
        {editable && settings && (
          <>
            <button
              type="button"
              disabled={busy || stale}
              className="flex min-h-10 w-full items-center justify-between gap-2 text-left disabled:opacity-50"
              onClick={() => setDialog('limits')}
            >
              <span className="text-zinc-500">运行限制</span>
              <span>
                {settings.runtime_limits.max_model_turns} 轮 ·{' '}
                {settings.runtime_limits.max_tool_calls} 次工具
              </span>
            </button>
            {time && (
              <button
                type="button"
                disabled={busy || stale}
                className="flex min-h-10 w-full items-center justify-between gap-2 text-left disabled:opacity-50"
                onClick={() => setDialog('time')}
              >
                <span className="text-zinc-500">时区</span>
                <span>{time.configuration?.timezone || 'Asia/Shanghai'}</span>
              </button>
            )}
            {dialog === 'limits' && (
              <RuntimeLimitsDialog
                limits={settings.runtime_limits}
                busy={busy}
                error={error}
                onClose={() => setDialog(undefined)}
                onSave={(runtimeLimits) => void save({ runtimeLimits })}
              />
            )}
            {dialog === 'time' && time && (
              <ToolConfigDialog
                tool={time}
                hasTimezone
                busy={busy}
                error={error}
                onClose={() => setDialog(undefined)}
                onSave={(changes) => {
                  if (Object.keys(changes).length)
                    void save({ tool: time, changes });
                  else setDialog(undefined);
                }}
              />
            )}
          </>
        )}
      </details>
    </>
  );
}
