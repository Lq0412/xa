import { Link } from 'umi';
import type { ApprovalRecord } from '@/services/xa/api';

/** 审批决定只用文字表达，黄色圆点仅表示等待用户确认。 */
export function approvalStatus(row: ApprovalRecord) {
  if (row.status === 'approved')
    return (
      {
        once: '已允许本次',
        conversation: '本对话已允许',
        agent: '此 Agent 已允许',
      }[row.remember] || '已允许'
    );
  return (
    {
      pending: '待确认',
      rejected: '已拒绝',
      expired: '超时未执行',
      cancelled: '已取消',
    }[row.status] || row.status
  );
}

/** 优先使用服务端脱敏后的业务摘要，不推断原始工具参数。 */
export function approvalTitle(row: ApprovalRecord) {
  return row.summary || row.title || row.tool_name;
}

export function ApprovalStatus({ row }: { row: ApprovalRecord }) {
  return (
    <span className="approval-status">
      {row.status === 'pending' && <span className="pending-dot" />}
      {approvalStatus(row)}
    </span>
  );
}

/** 今天页的紧凑记录直接进入对应详情。 */
export function ApprovalRows({ rows }: { rows: ApprovalRecord[] }) {
  return (
    <div className="summary-list">
      {rows.length ? (
        rows.map((row) => (
          <Link
            key={row.id}
            to={`/approvals?status=${row.status === 'pending' ? 'pending' : 'all'}&selected=${encodeURIComponent(row.id)}`}
            className="summary-row"
          >
            <span className="min-w-0">
              <span className="block truncate font-medium">
                {approvalTitle(row)}
              </span>
              <span className="mt-1.5 block truncate text-xs text-zinc-500">
                {row.agent} · {row.user}
              </span>
            </span>
            <ApprovalStatus row={row} />
            <time
              className="text-xs text-zinc-500 tabular-nums"
              dateTime={row.created_at}
            >
              {new Date(row.created_at).toLocaleTimeString('zh-CN', {
                hour: '2-digit',
                minute: '2-digit',
                hour12: false,
              })}
            </time>
          </Link>
        ))
      ) : (
        <p className="empty-state">暂无记录</p>
      )}
    </div>
  );
}
