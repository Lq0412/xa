import { useEffect, useState } from 'react';
import { Skeleton } from '@/components/ui/skeleton';
import { errorMessage, request } from '@/services/http';
import { approvalCopy, type PendingApproval } from './approval-copy';

type ActivityItem = {
  id: string;
  title: string;
  result: string;
  created_at: string;
};

type ActivityPage = { items: ActivityItem[]; next_cursor: string };

/** 日期分组使用浏览器的本地日期，与聊天时间一致。 */
export function panelTime(value: string) {
  const date = new Date(value);
  const today = new Date();
  const yesterday = new Date();
  yesterday.setDate(today.getDate() - 1);
  const day = date.toDateString();
  const group =
    day === today.toDateString()
      ? '今天'
      : day === yesterday.toDateString()
        ? '昨天'
        : '更早';
  const time =
    group === '更早'
      ? date.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric' })
      : date.toLocaleTimeString('zh-CN', {
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
        });
  return { group, time };
}

/** 动态页每次挂载重新读取，待确认项来自当前会话。 */
export function AgentActivity({
  agentId,
  approvals,
  onShowApproval,
  owned,
}: {
  agentId: string;
  approvals: PendingApproval[];
  onShowApproval: () => void;
  owned: boolean;
}) {
  const [page, setPage] = useState<ActivityPage>();
  const [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(owned);
  const [error, setError] = useState('');
  const [retry, setRetry] = useState(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: retry 重新读取同一页。
  useEffect(() => {
    if (!owned) return;
    const controller = new AbortController();
    setLoading(true);
    setError('');
    request<ActivityPage>(
      `/agents/${encodeURIComponent(agentId)}/activity?cursor=${encodeURIComponent(cursor)}`,
      { signal: controller.signal },
    )
      .then((next) => {
        if (controller.signal.aborted) return;
        setPage((current) =>
          cursor && current
            ? { ...next, items: [...current.items, ...next.items] }
            : next,
        );
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [agentId, cursor, owned, retry]);

  const items = (page?.items ?? []).map((item) => ({
    ...item,
    ...panelTime(item.created_at),
  }));
  const pending = approvals[0];
  return (
    <>
      {['今天', '昨天', '更早'].map((group) => {
        const rows = items.filter((item) => item.group === group);
        const pinned = group === '今天' ? pending : undefined;
        if (!pinned && !rows.length) return null;
        return (
          <section key={group} className="mb-3">
            <h3 className="px-4 pt-3 pb-2 text-xs text-zinc-500">{group}</h3>
            {pinned && (
              <button
                type="button"
                className="mx-2 flex w-[calc(100%-1rem)] justify-between gap-2 rounded-xl bg-[#fdf6df] px-2 py-2.5 text-left hover:bg-amber-100"
                onClick={onShowApproval}
              >
                <span className="min-w-0">
                  <span className="block truncate">
                    {pinned.name === 'memory_forget'
                      ? '想忘掉一条记忆'
                      : `想${approvalCopy(pinned).title}`}
                  </span>
                  <span className="mt-0.5 block truncate text-[13px] text-zinc-500">
                    {approvalCopy(pinned).subject ||
                      approvalCopy(pinned).action}
                  </span>
                </span>
                <span className="shrink-0 text-xs text-zinc-500">待确认</span>
              </button>
            )}
            {rows.map((item) => (
              <div key={item.id} className="px-4 py-2.5">
                <div className="flex items-baseline justify-between gap-2">
                  <p className="truncate" title={item.title}>
                    {item.title}
                  </p>
                  <time
                    dateTime={item.created_at}
                    className="shrink-0 text-xs tabular-nums text-zinc-500"
                  >
                    {item.time}
                  </time>
                </div>
                <p
                  className="mt-0.5 truncate text-[13px] text-zinc-500"
                  title={item.result}
                >
                  {item.result}
                </p>
              </div>
            ))}
          </section>
        );
      })}
      {loading && !page && <Skeleton className="m-4 h-20" />}
      {error && (
        <p role="alert" className="px-4 py-3 text-rose-700">
          {error}{' '}
          <button
            type="button"
            className="underline"
            onClick={() => setRetry((value) => value + 1)}
          >
            重试
          </button>
        </p>
      )}
      {!loading && !error && !pending && !items.length && (
        <p className="px-4 py-5 text-zinc-500">
          {owned ? '暂无动态' : '仅 Agent 所有者可查看动态'}
        </p>
      )}
      {page?.next_cursor && !error && (
        <button
          type="button"
          disabled={loading}
          className="mx-4 mb-4 text-xs text-zinc-600 hover:underline"
          onClick={() => setCursor(page.next_cursor)}
        >
          {loading ? '加载中…' : '加载更多'}
        </button>
      )}
    </>
  );
}
