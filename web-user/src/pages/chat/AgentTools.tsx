import { useEffect, useState } from 'react';
import { type Agent, getAgent, getAgentConfig } from '@/services/api';
import { errorMessage, request } from '@/services/http';

type ToolSetting = {
  name: string;
  title: string;
  enabled: boolean;
  approval: 'always' | 'never';
  available: boolean;
  approval_overridable: boolean;
};
type ToolBinding = {
  name: string;
  enabled: boolean;
  approval?: string;
  [key: string]: unknown;
};

/** 工具开关使用既有版本保存接口，保留未展示的绑定字段。 */
export function AgentTools({
  agent,
  onSaved,
}: {
  agent: Agent;
  onSaved: (agent: Agent) => void;
}) {
  const [tools, setTools] = useState<ToolSetting[]>([]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const changed = () => setRevision((value) => value + 1);
    window.addEventListener('xa:tools-updated', changed);
    return () => window.removeEventListener('xa:tools-updated', changed);
  }, []);
  // biome-ignore lint/correctness/useExhaustiveDependencies: 外部保存后按版本或通知重新读取。
  useEffect(() => {
    const controller = new AbortController();
    setError('');
    request<{ tools: ToolSetting[] }>(
      `/agents/${encodeURIComponent(agent.id)}/tools`,
      { signal: controller.signal },
    )
      .then((result) => {
        if (!controller.signal.aborted) setTools(result.tools);
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      });
    return () => controller.abort();
  }, [agent.id, agent.current_version_id, revision]);
  const toggle = async (tool: ToolSetting) => {
    setSaving(true);
    setError('');
    try {
      const version = await getAgentConfig(agent.id);
      const bindings = [
        ...((version.configuration.tools ?? []) as ToolBinding[]),
      ];
      const index = bindings.findIndex((item) => item.name === tool.name);
      const approval = tool.approval === 'always' ? 'never' : 'always';
      if (index >= 0) bindings[index] = { ...bindings[index], approval };
      else bindings.push({ name: tool.name, enabled: tool.enabled, approval });
      await request(`/agents/${encodeURIComponent(agent.id)}/tools`, {
        method: 'PUT',
        body: JSON.stringify({
          expected_version_id: version.id,
          tools: bindings,
        }),
      });
      onSaved(await getAgent(agent.id));
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setSaving(false);
    }
  };
  return (
    <section className="mt-4 space-y-2 text-left" aria-label="工具设置">
      <h3 className="text-sm font-medium">工具</h3>
      {tools
        .filter((tool) => tool.enabled && tool.available)
        .map((tool) => (
          <label
            key={tool.name}
            className="flex min-h-11 items-center justify-between gap-3 text-xs"
          >
            <span>{tool.title}</span>
            <span className="flex items-center gap-2">
              需要确认
              <input
                type="checkbox"
                role="switch"
                aria-checked={tool.approval === 'always'}
                aria-label={`${tool.title}需要确认`}
                checked={tool.approval === 'always'}
                disabled={saving || !tool.approval_overridable}
                onChange={() => void toggle(tool)}
              />
            </span>
          </label>
        ))}
      {error && (
        <p role="alert" className="text-xs text-rose-600">
          {error}
        </p>
      )}
    </section>
  );
}
