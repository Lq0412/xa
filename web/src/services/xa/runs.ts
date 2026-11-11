import { request } from './request';

/** 管理端运行摘要仅包含本 workspace 的脱敏查询字段。 */
export type RunSummary = {
  id: string;
  conversation_id: string;
  agent_id: string;
  agent_name: string;
  user_email: string;
  status: string;
  model: string;
  steps: number;
  tool_calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  cached_tokens: number;
  ttft_ms: number | null;
  latency_ms: number;
  active_ms: number;
  error_code: string;
  created_at: string;
  started_at: string | null;
  ended_at: string | null;
};

/** 详情不请求 run_contexts 或工具的原始载荷。 */
export type RunDetail = RunSummary & {
  workspace_id: string;
  owner_user_id: string;
  agent_version_id: string;
  user_message_id: string;
  final_message_id: string;
  provider: string;
  stream_protocol: number;
  limits: Record<string, unknown>;
  toolset_hash: string;
  toolset: string[];
  error: string;
  cancel_reason: string;
};

/** 服务端已按执行顺序组织树，前端保留原始顺序与脱敏 preview。 */
export type SpanNode = {
  id: string;
  kind: string;
  name: string;
  status: string;
  step: number | null;
  call_id: string;
  error_code: string;
  attributes: Record<string, unknown>;
  input_preview: string;
  output_preview: string;
  prompt_tokens: number | null;
  completion_tokens: number | null;
  cached_tokens: number | null;
  estimated_tokens: number | null;
  started_at: string;
  ended_at: string | null;
  duration_ms: number | null;
  children: SpanNode[];
};

/** 所有筛选均交给服务端，分页使用服务端返回的 keyset 游标。 */
export function listRuns(filters: Record<string, string>, signal: AbortSignal) {
  const query = new URLSearchParams({ limit: '20' });
  for (const [key, value] of Object.entries(filters)) {
    if (value) query.set(key, value);
  }
  return request<{ runs: RunSummary[]; next_cursor: string }>(
    `/api/v1/admin/runs?${query}`,
    { signal },
  );
}

/** 运行摘要与树分别读取，旧运行可以合法返回空树。 */
export async function getRun(id: string, signal: AbortSignal) {
  const path = `/api/v1/admin/runs/${encodeURIComponent(id)}`;
  const [run, trace] = await Promise.all([
    request<RunDetail>(path, { signal }),
    request<{ run_id: string; span: SpanNode | null }>(`${path}/spans`, {
      signal,
    }),
  ]);
  return { run, span: trace.span };
}
