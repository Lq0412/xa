import { getAgentConfig } from './api';
import { ApiError, request } from './http';

export type ToolApproval = 'always' | 'never';

/** 工具目录中的说明与当前前端支持的配置字段。 */
type ToolSpec = {
  name: string;
  description: string;
  config_schema: { properties?: { timezone?: { type: string } } };
};

/** 版本解析后的工具状态，禁用原因与绑定本身的开关分别处理。 */
export type AgentTool = {
  name: string;
  title: string;
  enabled: boolean;
  approval: ToolApproval;
  available: boolean;
  reason: string;
  approval_overridable: boolean;
  max_calls: number;
  timeout_ms: number;
  configuration: { timezone?: string };
};

/** 写入时保留原始绑定中未编辑的字段。 */
export type ToolBinding = {
  name: string;
  enabled: boolean;
  approval?: ToolApproval;
  timeout_ms?: number;
  max_calls?: number;
  configuration?: Record<string, unknown>;
  [key: string]: unknown;
};

export type RuntimeLimits = {
  max_model_turns: number;
  max_tool_calls: number;
  max_parallel_tools: number;
  [key: string]: number;
};

export type AgentToolSettings = {
  current_version_id: string;
  tools: AgentTool[];
  runtime_limits: RuntimeLimits;
};

export const getToolCatalog = (signal: AbortSignal) =>
  request<{ tools: ToolSpec[] }>('/tools', { signal });

const toolSettingsPath = (agentId: string) =>
  `/agents/${encodeURIComponent(agentId)}/tools`;

export const getAgentTools = (agentId: string, signal: AbortSignal) =>
  request<AgentToolSettings>(toolSettingsPath(agentId), { signal });

/** 只用显示时的版本保存，读取原始绑定时同样检查版本。 */
export async function saveAgentTools(
  agentId: string,
  expectedVersionId: string,
  edit:
    | { tool: AgentTool; changes: Partial<ToolBinding> }
    | { runtimeLimits: RuntimeLimits },
) {
  const version = await getAgentConfig(agentId);
  if (version.id !== expectedVersionId)
    throw new ApiError(409, '配置已被更新，请刷新');

  const bindings = (version.configuration.tools ?? []) as ToolBinding[];
  let tools = bindings;
  if ('tool' in edit) {
    const { tool, changes } = edit;
    const existing = bindings.find((binding) => binding.name === tool.name);
    const updated = {
      name: tool.name,
      enabled: tool.enabled,
      ...existing,
      ...changes,
    };
    if (changes.configuration) {
      updated.configuration = {
        ...existing?.configuration,
        ...changes.configuration,
      };
    }
    tools = existing
      ? bindings.map((binding) => (binding === existing ? updated : binding))
      : [...bindings, updated];
  }

  return request<AgentToolSettings>(toolSettingsPath(agentId), {
    method: 'PUT',
    body: JSON.stringify({
      expected_version_id: expectedVersionId,
      tools,
      runtime_limits: 'runtimeLimits' in edit ? edit.runtimeLimits : undefined,
    }),
  });
}
