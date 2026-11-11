import * as runs from '../generated/runs';

/** 管理端运行摘要仅包含本 workspace 的脱敏查询字段。 */
export type RunSummary = NonNullable<
  Awaited<ReturnType<typeof runs.listAdminRuns>>['runs']
>[number];

/** 详情保留服务端提供的脱敏字段。 */
export type RunDetail = Awaited<ReturnType<typeof runs.getAdminRun>>;

/** 服务端已按执行顺序组织树，前端保留顺序与脱敏 preview。 */
export type SpanNode = NonNullable<
  Awaited<ReturnType<typeof runs.getAdminRunSpans>>['span']
>;

/** 所有筛选均交给服务端，分页使用服务端返回的 keyset 游标。 */
export function listRuns(filters: Record<string, string>, signal: AbortSignal) {
  return runs.listAdminRuns(
    {
      limit: '20',
      ...Object.fromEntries(
        Object.entries(filters).filter(([, value]) => value),
      ),
    },
    { signal },
  );
}

/** 运行摘要与树分别读取，旧运行可以合法返回空树。 */
export async function getRun(id: string, signal: AbortSignal) {
  const params = { run_id: encodeURIComponent(id) };
  const [run, trace] = await Promise.all([
    runs.getAdminRun(params, { signal }),
    runs.getAdminRunSpans(params, { signal }),
  ]);
  return { run, span: trace.span };
}
