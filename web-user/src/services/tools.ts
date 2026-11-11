import { getAgentConfig } from './api';
import * as toolAPI from './generated/tools';
import { ApiError } from './http';

/** 版本解析后的工具状态，禁用原因与绑定本身的开关分别处理。 */
export type AgentTool = NonNullable<AgentToolSettings['tools']>[number];

/** 写入时保留原始绑定中未编辑的字段。 */
export type ToolBinding = API.ToolBinding;

export type RuntimeLimits = AgentToolSettings['runtime_limits'];

export type AgentToolSettings = Awaited<
  ReturnType<typeof toolAPI.getAgentTools>
>;

export const getToolCatalog = (signal: AbortSignal) =>
  toolAPI.listTools({ signal });

export const getAgentTools = (agentId: string, signal: AbortSignal) =>
  toolAPI.getAgentTools({ agent_id: encodeURIComponent(agentId) }, { signal });

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

  const bindings = version.configuration.tools ?? [];
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

  return toolAPI.updateAgentTools(
    { agent_id: encodeURIComponent(agentId) },
    {
      expected_version_id: expectedVersionId,
      tools,
      runtime_limits: 'runtimeLimits' in edit ? edit.runtimeLimits : undefined,
    },
  );
}
