import {
  ArrowLeft,
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  RefreshCw,
} from 'lucide-react';
import { useCallback } from 'react';
import { useSearchParams } from 'umi';
import {
  ApprovalStatus,
  approvalStatus,
  approvalTitle,
} from '@/components/ApprovalSummary';
import { ErrorNotice } from '@/components/Page';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useResource } from '@/lib/useResource';
import { listApprovals, listTools } from '@/services/xa/api';

function dateGroup(value: string) {
  const date = new Date(value).toDateString();
  const today = new Date();
  if (date === today.toDateString()) return '今天';
  today.setDate(today.getDate() - 1);
  return date === today.toDateString() ? '昨天' : '更早';
}

const loadTools = () => listTools();

/** 只读排查页，筛选由后端完成；URL 保存选中项与游标以支持刷新。 */
export default function ApprovalsPage() {
  const [params, setParams] = useSearchParams();
  const status = params.get('status') === 'all' ? '' : 'pending';
  const tool = params.get('tool') || '';
  const cursor = params.get('cursor') || '';
  const selectedId = params.get('selected') || '';
  const load = useCallback(
    async (signal: AbortSignal) => {
      const result = await listApprovals(
        { status, tool_name: tool, cursor },
        signal,
      );
      // 从今天页进入时，目标可能已经排到下一页；继续读取直到找到它。
      while (
        !cursor &&
        selectedId &&
        result.next_cursor &&
        !result.approvals.some((row) => row.id === selectedId)
      ) {
        const next = await listApprovals(
          { status, tool_name: tool, cursor: result.next_cursor },
          signal,
        );
        result.approvals.push(...next.approvals);
        result.next_cursor = next.next_cursor;
      }
      return result;
    },
    [status, tool, cursor, selectedId],
  );
  const { data, loading, error, reload } = useResource(load);
  const catalog = useResource(loadTools);
  const selected =
    data?.approvals.find((row) => row.id === selectedId) ?? data?.approvals[0];
  function change(values: Record<string, string>) {
    const next = new URLSearchParams(values);
    setParams(next);
  }
  let previous = '';
  return (
    <div className={selectedId ? 'has-selection' : ''}>
      <header className="approval-header">
        <div className="approval-heading">
          <h1>确认记录</h1>
          <div className="flex items-center gap-4">
            <select
              aria-label="筛选工具"
              className="plain-select"
              value={tool}
              disabled={catalog.loading || !!catalog.error}
              onChange={(event) =>
                change({
                  status: status ? 'pending' : 'all',
                  tool: event.target.value,
                })
              }
            >
              <option value="">全部工具</option>
              {catalog.data?.tools.map((item) => (
                <option key={item.name} value={item.name}>
                  {item.title}
                </option>
              ))}
            </select>
            <Button
              variant="ghost"
              size="sm"
              aria-label="刷新确认记录"
              onClick={reload}
              disabled={loading}
            >
              <RefreshCw size={14} />
            </Button>
          </div>
        </div>
        <div className="text-tabs">
          <button
            type="button"
            aria-pressed={!!status}
            onClick={() => change({ status: 'pending', tool })}
          >
            待确认
          </button>
          <button
            type="button"
            aria-pressed={!status}
            onClick={() => change({ status: 'all', tool })}
          >
            全部
          </button>
        </div>
      </header>
      {catalog.error && (
        <div className="p-6">
          <ErrorNotice>{catalog.error}</ErrorNotice>
          <Button variant="ghost" onClick={catalog.reload}>
            重试
          </Button>
        </div>
      )}
      <div className="approval-columns">
        <section
          className="approval-list"
          aria-label="确认记录列表"
          aria-busy={loading}
        >
          {error ? (
            <ErrorNotice>{error}</ErrorNotice>
          ) : loading ? (
            <div className="space-y-4 p-3">
              {[1, 2, 3].map((i) => (
                <Skeleton key={i} className="h-16" />
              ))}
            </div>
          ) : data?.approvals.length ? (
            data.approvals.map((row) => {
              const group = dateGroup(row.created_at);
              const heading = group !== previous;
              previous = group;
              return (
                <div key={row.id}>
                  {heading && <h2 className="date-group">{group}</h2>}
                  <button
                    type="button"
                    className="approval-record"
                    aria-current={selected?.id === row.id}
                    onClick={() =>
                      change({
                        status: status ? 'pending' : 'all',
                        tool,
                        cursor,
                        selected: row.id,
                      })
                    }
                  >
                    <span className="record-title">
                      <span
                        className={
                          row.status === 'pending'
                            ? 'pending-dot'
                            : 'pending-dot resolved-dot'
                        }
                      />
                      <span className="min-w-0 flex-1 break-words">
                        {approvalTitle(row)}
                      </span>
                      <time dateTime={row.created_at}>
                        {new Date(row.created_at).toLocaleTimeString('zh-CN', {
                          hour: '2-digit',
                          minute: '2-digit',
                          hour12: false,
                        })}
                      </time>
                    </span>
                    <span className="record-meta">
                      <span className="min-w-0 truncate">
                        {row.agent} · {row.user}
                      </span>
                      {row.status !== 'pending' && (
                        <span className="ml-auto shrink-0">
                          {approvalStatus(row)}
                        </span>
                      )}
                    </span>
                  </button>
                </div>
              );
            })
          ) : (
            <p className="empty-state px-3">
              {status ? '暂无待确认记录' : '暂无确认记录'}
            </p>
          )}
          <div className="mt-5 flex justify-between px-3">
            {cursor && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  change({ status: status ? 'pending' : 'all', tool })
                }
              >
                <ChevronLeft size={14} />
                返回第一页
              </Button>
            )}
            {data?.next_cursor && (
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto"
                onClick={() =>
                  change({
                    status: status ? 'pending' : 'all',
                    tool,
                    cursor: data.next_cursor,
                  })
                }
              >
                下一页
                <ChevronRight size={14} />
              </Button>
            )}
          </div>
        </section>
        <article className="approval-detail" aria-label="记录详情">
          <button
            type="button"
            className="detail-back"
            onClick={() =>
              change({ status: status ? 'pending' : 'all', tool, cursor })
            }
          >
            <ArrowLeft size={14} />
            确认记录
          </button>
          {loading ? (
            <Skeleton className="h-40" />
          ) : selected ? (
            <>
              <div className="flex items-center justify-between">
                <ApprovalStatus row={selected} />
                <span className="text-[11px] text-zinc-400">只读</span>
              </div>
              <h2>{approvalTitle(selected)}</h2>
              {selected.status === 'pending' && (
                <p className="text-xs text-zinc-500">由用户在对话中确认</p>
              )}
              <dl className="detail-fields">
                <dt>用户</dt>
                <dd>{selected.user}</dd>
                <dt>Agent</dt>
                <dd>{selected.agent}</dd>
                <dt>工具</dt>
                <dd className="font-mono text-xs text-zinc-600">
                  {selected.tool_name}
                </dd>
                <dt>发起时间</dt>
                <dd>
                  {new Date(selected.created_at).toLocaleString('zh-CN', {
                    hour12: false,
                  })}
                </dd>
                <dt>等待时长</dt>
                <dd className="tabular-nums">
                  {Math.ceil(selected.wait_ms / 1000)} 秒
                </dd>
                {selected.decider && (
                  <>
                    <dt>决定人</dt>
                    <dd>{selected.decider}</dd>
                  </>
                )}
              </dl>
              <details className="config-disclosure border-t border-zinc-100">
                <summary>
                  记录编号
                  <ChevronRight size={14} className="text-zinc-400" />
                </summary>
                <dl className="detail-fields mt-0">
                  <dt>审批</dt>
                  <dd className="font-mono text-xs">{selected.id}</dd>
                  <dt>会话</dt>
                  <dd className="font-mono text-xs">
                    {selected.conversation_id}
                  </dd>
                  <dt>轮次</dt>
                  <dd className="font-mono text-xs">{selected.run_id}</dd>
                </dl>
              </details>
              <button
                type="button"
                disabled
                className="mt-6 flex items-center gap-2 text-xs text-zinc-400"
                title="运行追踪即将上线"
              >
                查看这次运行
                <ArrowUpRight size={14} />
              </button>
            </>
          ) : (
            <p className="empty-state">选择一条记录</p>
          )}
        </article>
      </div>
    </div>
  );
}
