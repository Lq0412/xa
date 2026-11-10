import {
  type ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';
import { Skeleton } from './ui/skeleton';

/** 表格直接铺在页面上，仅用行间浅线分隔；名称链接提供键盘入口。 */
export function DataTable<T extends { id: string }>({
  label,
  rows,
  columns,
  loading = false,
  onOpen,
}: {
  label: string;
  rows: T[];
  columns: ColumnDef<T>[];
  loading?: boolean;
  onOpen?: (row: T) => void;
}) {
  const table = useReactTable({
    columns,
    data: rows,
    getRowId: (row) => row.id,
    getCoreRowModel: getCoreRowModel(),
  });
  return (
    <div className="overflow-x-auto">
      <table aria-label={label} aria-busy={loading} className="admin-table">
        <thead>
          {table.getHeaderGroups().map((group) => (
            <tr key={group.id}>
              {group.headers.map((header) => (
                <th
                  key={header.id}
                  scope="col"
                  style={{ width: header.column.columnDef.size }}
                >
                  {flexRender(
                    header.column.columnDef.header,
                    header.getContext(),
                  )}
                </th>
              ))}
            </tr>
          ))}
        </thead>
        <tbody>
          {loading
            ? [1, 2, 3].map((row) => (
                <tr key={row}>
                  {columns.map((column, index) => (
                    <td key={column.id || String(index)}>
                      <Skeleton className="h-4 w-3/4" />
                    </td>
                  ))}
                </tr>
              ))
            : table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  className={onOpen ? 'cursor-pointer' : undefined}
                  onClick={(event) => {
                    if (
                      !event.ctrlKey &&
                      !event.metaKey &&
                      !(event.target as HTMLElement).closest('a,button')
                    )
                      onOpen?.(row.original);
                  }}
                  onKeyDown={(event) => {
                    if (
                      event.target === event.currentTarget &&
                      event.key === 'Enter'
                    )
                      onOpen?.(row.original);
                  }}
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </td>
                  ))}
                </tr>
              ))}
        </tbody>
      </table>
    </div>
  );
}

/** 状态标签不依赖颜色传递含义。 */
export function Badge({
  children,
  tone = 'neutral',
}: {
  children: ReactNode;
  tone?: 'neutral' | 'green';
}) {
  return (
    <span
      className={cn(
        'inline-block rounded px-1.5 py-0.5 text-[11px] whitespace-nowrap',
        tone === 'green'
          ? 'bg-emerald-50 text-emerald-700'
          : 'bg-zinc-100 text-zinc-500',
      )}
    >
      {children}
    </span>
  );
}

/** 统一显示本地时间，保留完整时间供悬停查看。 */
export function TimeCell({ value }: { value: string }) {
  return (
    <time
      dateTime={value}
      title={new Date(value).toLocaleString('zh-CN')}
      className="text-xs whitespace-nowrap text-zinc-500 tabular-nums"
    >
      {new Date(value).toLocaleDateString('zh-CN', {
        month: '2-digit',
        day: '2-digit',
      })}{' '}
      {new Date(value).toLocaleTimeString('zh-CN', {
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
      })}
    </time>
  );
}
