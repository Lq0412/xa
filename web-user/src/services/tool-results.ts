import { getToolResult as getToolResultRequest } from './generated/runs';

/** 完整工具载荷只通过当前对话所有者的接口按需读取。 */
export type ToolResult = Awaited<ReturnType<typeof getToolResultRequest>>;

export function getToolResult(
  runId: string,
  callId: string,
  signal: AbortSignal,
) {
  return getToolResultRequest(
    { run_id: encodeURIComponent(runId), call_id: encodeURIComponent(callId) },
    { signal },
  );
}
