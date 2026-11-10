import { RefreshCw } from 'lucide-react';
import { Link } from 'umi';
import { ApprovalRows } from '@/components/ApprovalSummary';
import { Badge } from '@/components/DataTable';
import { ErrorNotice, Page } from '@/components/Page';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useResource } from '@/lib/useResource';
import { allApprovals, listAgents } from '@/services/xa/api';

async function loadOverview(signal: AbortSignal) {
  const [agents, approvals] = await Promise.all([
    listAgents(signal),
    allApprovals(signal),
  ]);
  return { agents: agents.agents ?? [], approvals };
}

/** 统计以本地自然日和决定时间为准，待确认只计当前状态。 */
export default function OverviewPage() {
  const { data, loading, error, reload } = useResource(loadOverview);
  const midnight = new Date();
  midnight.setHours(0, 0, 0, 0);
  const pending =
    data?.approvals.filter((row) => row.status === 'pending') ?? [];
  const decided =
    data?.approvals.filter(
      (row) =>
        row.status !== 'pending' &&
        new Date(row.created_at).getTime() + row.wait_ms >= midnight.getTime(),
    ) ?? [];
  const publicAgents =
    data?.agents.filter((agent) => agent.visibility === 'public') ?? [];
  const average = decided.length
    ? `${Math.round(decided.reduce((sum, row) => sum + row.wait_ms, 0) / decided.length / 1000)} 秒`
    : '—';
  const recent = (
    data?.approvals.filter((row) => row.status !== 'pending') ?? []
  ).sort(
    (a, b) =>
      new Date(b.created_at).getTime() +
      b.wait_ms -
      (new Date(a.created_at).getTime() + a.wait_ms),
  );
  return (
    <Page
      title="今天"
      actions={
        <div className="flex items-center gap-4">
          <time className="text-xs text-zinc-500">
            {new Date().toLocaleDateString('zh-CN', {
              month: 'long',
              day: 'numeric',
              weekday: 'long',
            })}
          </time>
          <Button
            variant="ghost"
            size="sm"
            aria-label="刷新概览"
            onClick={reload}
            disabled={loading}
          >
            <RefreshCw size={14} />
          </Button>
        </div>
      }
    >
      {error ? (
        <ErrorNotice>{error}</ErrorNotice>
      ) : loading ? (
        <div aria-busy="true" className="space-y-10">
          <div className="grid grid-cols-4 gap-8">
            {[1, 2, 3, 4].map((i) => (
              <Skeleton key={i} className="h-20" />
            ))}
          </div>
          <Skeleton className="h-48" />
        </div>
      ) : (
        <>
          <dl className="metrics">
            {[
              [String(pending.length), '待确认'],
              [
                String(
                  publicAgents.filter((agent) => agent.status === 'active')
                    .length,
                ),
                '公共 Agent · 已启用',
              ],
              [String(decided.length), '今日已处理'],
              [average, '今日平均等待'],
            ].map(([value, label]) => (
              <div key={label}>
                <dd>{value}</dd>
                <dt>{label}</dt>
              </div>
            ))}
          </dl>
          <div className="overview-sections">
            <section>
              <div className="section-heading">
                <h2>等待确认</h2>
                <Link to="/approvals">查看全部</Link>
              </div>
              <ApprovalRows rows={pending.slice(0, 5)} />
            </section>
            <section>
              <div className="section-heading">
                <h2>公共 Agent</h2>
                <Link to="/agents">查看全部</Link>
              </div>
              {publicAgents.length ? (
                publicAgents.slice(0, 5).map((agent) => (
                  <Link
                    key={agent.id}
                    to={`/agents/${encodeURIComponent(agent.id)}`}
                    className="flex items-center gap-3 border-b border-zinc-100 py-4"
                  >
                    <img
                      src={agent.avatar || '/default-agent-avatar.png'}
                      alt=""
                      className="agent-avatar"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[13px] font-medium">
                        {agent.name}
                      </span>
                      {agent.description && (
                        <span className="mt-1 block truncate text-xs text-zinc-500">
                          {agent.description}
                        </span>
                      )}
                    </span>
                    <Badge
                      tone={agent.status === 'active' ? 'green' : 'neutral'}
                    >
                      {agent.status === 'active' ? '已启用' : '已停用'}
                    </Badge>
                  </Link>
                ))
              ) : (
                <p className="empty-state">暂无公共 Agent</p>
              )}
            </section>
            <section>
              <div className="section-heading">
                <h2>最近确认</h2>
                <Link to="/approvals?status=all">查看全部</Link>
              </div>
              <ApprovalRows rows={recent.slice(0, 5)} />
            </section>
          </div>
        </>
      )}
    </Page>
  );
}
