/** 状态文本同时用于列表、摘要与执行节点，保留未知状态供排查。 */
export function statusText(value: string) {
  return (
    (
      {
        queued: '排队中',
        running: '运行中',
        waiting_approval: '等待确认',
        completed: '已完成',
        failed: '失败',
        cancelled: '已取消',
        interrupted: '已中断',
        budget_exhausted: '已达预算',
        ok: '成功',
        error: '失败',
      } as Record<string, string>
    )[value] || value
  );
}

/** 毫秒为零代表已完成的零耗时，只有 null 表示尚无记录。 */
export function duration(value: number | null) {
  if (value === null) return '—';
  if (value < 1000) return `${value} ms`;
  return `${(value / 1000).toLocaleString('zh-CN', { maximumFractionDigits: 2 })} s`;
}

/** 时间缺失时不推算，避免将排队时间当作执行时间。 */
export function dateTime(value: string | null) {
  return value
    ? new Date(value).toLocaleString('zh-CN', { hour12: false })
    : '—';
}

/** 失败仅改变文字颜色，不给整行增加警告底色。 */
export function RunStatus({ value }: { value: string }) {
  return (
    <span
      className={
        value === 'failed' || value === 'error'
          ? 'text-rose-600'
          : value === 'running' || value === 'waiting_approval'
            ? 'text-amber-700'
            : 'text-zinc-500'
      }
    >
      {statusText(value)}
    </span>
  );
}
