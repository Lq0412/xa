import type { ColumnDef } from '@tanstack/react-table';
import { Plus, RefreshCw, Search } from 'lucide-react';
import { useState } from 'react';
import { history, Link } from 'umi';
import { Badge, DataTable, TimeCell } from '@/components/DataTable';
import { ErrorNotice, Page } from '@/components/Page';
import { Button } from '@/components/ui/button';
import { useResource } from '@/lib/useResource';
import { listAgents } from '@/services/xa/api';

const columns: ColumnDef<XA.Agent>[] = [
  {
    header: '名称',
    accessorKey: 'name',
    size: 240,
    cell: ({ row: { original: agent } }) => (
      <Link
        to={`/agents/${encodeURIComponent(agent.id)}`}
        className="agent-link"
      >
        <img
          src={agent.avatar || '/default-agent-avatar.png'}
          alt=""
          className="agent-avatar"
        />
        <span className="min-w-0">
          <span className="block truncate font-medium">{agent.name}</span>
          {agent.is_primary && agent.name !== '主 Agent' && (
            <span className="mt-1 block text-[11px] text-zinc-400">
              主 Agent
            </span>
          )}
        </span>
      </Link>
    ),
  },
  {
    header: '简介',
    accessorKey: 'description',
    size: 350,
    cell: ({ row }) => (
      <span
        className="block truncate text-zinc-500"
        title={row.original.description}
      >
        {row.original.description || '—'}
      </span>
    ),
  },
  {
    header: '状态',
    accessorKey: 'status',
    size: 100,
    cell: ({ row }) => (
      <Badge tone={row.original.status === 'active' ? 'green' : 'neutral'}>
        {row.original.status === 'active' ? '已启用' : '已停用'}
      </Badge>
    ),
  },
  {
    header: '更新时间',
    accessorKey: 'updated_at',
    size: 120,
    cell: ({ row }) => <TimeCell value={row.original.updated_at} />,
  },
];

/** 公共和自己的 Agent 分组展示，筛选针对接口返回的完整可见列表。 */
export default function AgentsPage() {
  const { data, loading, error, reload } = useResource(listAgents);
  const [search, setSearch] = useState('');
  const [visibility, setVisibility] = useState('');
  const [status, setStatus] = useState('');
  const filtered = (data?.agents ?? []).filter(
    (agent) =>
      (!search ||
        agent.name
          .toLocaleLowerCase()
          .includes(search.trim().toLocaleLowerCase())) &&
      (!visibility || agent.visibility === visibility) &&
      (!status || agent.status === status),
  );
  return (
    <Page
      title="Agent"
      actions={
        <div className="page-actions">
          <label className="search-field">
            <Search size={15} aria-hidden="true" />
            <input
              aria-label="搜索 Agent"
              placeholder="搜索 Agent"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          <select
            aria-label="可见范围"
            className="plain-select"
            value={visibility}
            onChange={(event) => setVisibility(event.target.value)}
          >
            <option value="">全部范围</option>
            <option value="public">公共</option>
            <option value="private">我的</option>
          </select>
          <Button asChild size="sm">
            <Link to="/agents/new">
              <Plus size={15} />
              新建 Agent
            </Link>
          </Button>
        </div>
      }
    >
      <div className="flex items-start justify-between border-b border-zinc-200">
        <div className="text-tabs border-0">
          {[
            ['', '全部'],
            ['active', '已启用'],
            ['offline', '已停用'],
          ].map(([value, label]) => (
            <button
              type="button"
              key={value}
              aria-pressed={status === value}
              onClick={() => setStatus(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <Button
          variant="ghost"
          size="sm"
          aria-label="刷新 Agent 列表"
          disabled={loading}
          onClick={reload}
        >
          <RefreshCw size={14} />
        </Button>
      </div>
      {error ? (
        <div className="mt-6">
          <ErrorNotice>{error}</ErrorNotice>
        </div>
      ) : loading ? (
        <DataTable label="Agent 列表" columns={columns} rows={[]} loading />
      ) : (
        <div className="agent-groups">
          {(['public', 'private'] as const).map((group) => {
            const rows = filtered.filter((agent) => agent.visibility === group);
            return rows.length ? (
              <section key={group}>
                <h2 className="agent-group-title">
                  {group === 'public' ? '公共' : '我的'}
                  <span className="ml-2 text-zinc-400">{rows.length}</span>
                </h2>
                <DataTable
                  label={group === 'public' ? '公共 Agent' : '我的 Agent'}
                  columns={columns}
                  rows={rows}
                  onOpen={(agent) =>
                    history.push(`/agents/${encodeURIComponent(agent.id)}`)
                  }
                />
              </section>
            ) : null;
          })}
          {!filtered.length && (
            <p className="empty-state">没有符合条件的 Agent</p>
          )}
        </div>
      )}
    </Page>
  );
}
