import { request } from './http';

/** 完整工具载荷只通过当前对话所有者的接口按需读取。 */
export type ToolResult = {
  run_id: string;
  call_id: string;
  tool_name: string;
  status: string;
  arguments: unknown;
  data: unknown;
};

export function getToolResult(
  runId: string,
  callId: string,
  signal: AbortSignal,
) {
  return request<ToolResult>(
    `/runs/${encodeURIComponent(runId)}/tool-results/${encodeURIComponent(callId)}`,
    { signal },
  );
}
