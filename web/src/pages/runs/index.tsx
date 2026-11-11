import type { ColumnDef } from '@tanstack/react-table';
import { ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import { useCallback } from 'react';
import { Link, useSearchParams } from 'umi';
import { DataTable, TimeCell } from '@/components/DataTable';
import { ErrorNotice, Page } from '@/components/Page';
import { Button } from '@/components/ui/button';
import { useResource } from '@/lib/useResource';
import { listAgents } from '@/services/xa/api';
import { listRuns, type RunSummary } from '@/services/xa/runs';
import { duration, RunStatus, statusText } from './format';

const loadAgents = (signal: AbortSignal) => listAgents(signal);
const statuses = [
  'queued',
  'running',
  'waiting_approval',
  'completed',
  'failed',
  'cancelled',
  'interrupted',
  'budget_exhausted',
];

/** 管理员按会话查询运行记录，链接保留其他筛选并清除旧游标。 */
function ConversationLink({ id }: { id: string }) {
  const [params] = useSearchParams();
  const next = new URLSearchParams(params);
  next.delete('cursor');
  next.set('conversation_id', id);
  return (
    <Link
      to={`/runs?${next}`}
      title={id}
      aria-label={`筛选会话 ${id}`}
      className="whitespace-nowrap font-mono text-xs text-zinc-500 hover:underline"
    >
      {id.length > 16 ? `${id.slice(0, 12)}…` : id}
    </Link>
  );
}
const columns: ColumnDef<RunSummary>[] = [
  {
    id: 'time',
    header: '时间',
    size: 112,
    cell: ({ row }) => (
      <Link
        className="hover:underline"
        to={`/runs/${encodeURIComponent(row.original.id)}`}
      >
        <TimeCell value={row.original.created_at} />
      </Link>
    ),
  },
  { accessorKey: 'user_email', header: '用户', size: 188 },
  { accessorKey: 'agent_name', header: 'Agent', size: 176 },
  {
    id: 'status',
    header: '状态',
    size: 104,
    cell: ({ row }) => <RunStatus value={row.original.status} />,
  },
  { accessorKey: 'steps', header: '步数', size: 64 },
  { accessorKey: 'tool_calls', header: '工具', size: 64 },
  {
    id: 'tokens',
    header: 'Tokens',
    size: 96,
    cell: ({ row }) =>
      (
        row.original.prompt_tokens + row.original.completion_tokens
      ).toLocaleString('zh-CN'),
  },
  {
    id: 'duration',
    header: '耗时',
    size: 96,
    cell: ({ row }) => duration(row.original.latency_ms),
  },
  {
    id: 'conversation',
    header: '会话',
    size: 136,
    cell: ({ row }) => <ConversationLink id={row.original.conversation_id} />,
  },
  {
    id: 'open',
    header: '',
    size: 64,
    cell: ({ row }) => (
      <Link
        to={`/runs/${encodeURIComponent(row.original.id)}`}
        className="text-xs hover:underline"
        aria-label={`查看运行 ${row.original.id}`}
      >
        查看
      </Link>
    ),
  },
];

/** 筛选和游标保存在 URL，刷新与从详情返回均保留查询条件。 */
export default function RunsPage() {
  const [params, setParams] = useSearchParams();
  const status = params.get('status') || '';
  const agent = params.get('agent_id') || '';
  const user = params.get('user_id') || '';
  const conversation = params.get('conversation_id') || '';
  const cursor = params.get('cursor') || '';
  const load = useCallback(
    (signal: AbortSignal) =>
      listRuns(
        {
          status,
          agent_id: agent,
          user_id: user,
          conversation_id: conversation,
          cursor,
        },
        signal,
      ),
    [status, agent, user, conversation, cursor],
  );
  const { data, loading, error, reload } = useResource(load);
  const agents = useResource(loadAgents);
  function change(key: string, value: string) {
    const next = new URLSearchParams(params);
    next.delete('cursor');
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next);
  }
  return (
    <Page
      title="运行追踪"
      actions={
        <div className="page-actions">
          <select
            aria-label="Agent"
            className="plain-select"
            value={agent}
            disabled={agents.loading || !!agents.error}
            onChange={(event) => change('agent_id', event.target.value)}
          >
            <option value="">全部 Agent</option>
            {agent &&
              !agents.data?.agents?.some((item) => item.id === agent) && (
                <option value={agent}>{agent}</option>
              )}
            {agents.data?.agents?.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
          <Button
            variant="ghost"
            size="sm"
            aria-label="刷新运行追踪"
            onClick={reload}
            disabled={loading}
          >
            <RefreshCw size={14} />
          </Button>
        </div>
      }
    >
      <div className="mb-6 border-b border-zinc-200">
        <fieldset
          className="text-tabs flex-wrap gap-x-6 gap-y-2"
          aria-label="运行状态"
        >
          {['', ...statuses].map((item) => (
            <button
              key={item}
              type="button"
              aria-pressed={status === item}
              onClick={() => change('status', item)}
            >
              {item ? statusText(item) : '全部'}
            </button>
          ))}
        </fieldset>
      </div>
      {(user || conversation) && (
        <Button
          variant="ghost"
          size="sm"
          className="mb-4"
          onClick={() =>
            setParams(
              status || agent
                ? {
                    ...(status && { status }),
                    ...(agent && { agent_id: agent }),
                  }
                : {},
            )
          }
        >
          清除用户与会话筛选
        </Button>
      )}
      {agents.error && (
        <div className="mb-4 space-y-2">
          <ErrorNotice>{agents.error}</ErrorNotice>
          <Button variant="ghost" size="sm" onClick={agents.reload}>
            重试 Agent 列表
          </Button>
        </div>
      )}
      {error ? (
        <div className="space-y-3">
          <ErrorNotice>{error}</ErrorNotice>
          <Button variant="ghost" onClick={reload}>
            重试
          </Button>
        </div>
      ) : (
        <>
          <div className="runs-table">
            <DataTable
              label="运行列表"
              rows={data?.runs || []}
              columns={columns}
              loading={loading}
            />
          </div>
          {!loading && !data?.runs.length && (
            <p className="empty-state">暂无符合条件的运行</p>
          )}
          <div className="mt-6 flex justify-between">
            {cursor && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => change('cursor', '')}
                disabled={loading}
              >
                <ChevronLeft size={14} />
                最新
              </Button>
            )}
            {data?.next_cursor && (
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto"
                onClick={() => change('cursor', data.next_cursor)}
                disabled={loading}
              >
                下一页
                <ChevronRight size={14} />
              </Button>
            )}
          </div>
        </>
      )}
    </Page>
  );
}
