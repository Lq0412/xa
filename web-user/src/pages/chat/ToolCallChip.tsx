import { Icon } from '@/components/Icon';
import type { ToolPart } from '@/services/chat-parts';

export function ToolCallChip({ part }: { part: ToolPart }) {
  const running =
    part.status === 'preparing' ||
    part.status === 'pending' ||
    part.status === 'running';
  const ok = part.status === 'succeeded';
  const failed = part.status === 'failed' || part.status === 'unknown';
  const chipStatus = running ? 'running' : ok ? 'done' : 'failed';
  return (
    <div
      className="my-2 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm shadow-sm"
      data-testid="tool-call-chip"
      data-status={chipStatus}
    >
      <div className="flex items-center gap-2">
        {running ? (
          <span className="inline-block h-3 w-3 animate-spin rounded-full border border-zinc-400 border-t-transparent" />
        ) : ok ? (
          <Icon name="check" className="text-emerald-600" />
        ) : failed ? (
          <Icon name="info" className="text-amber-600" />
        ) : null}
        <span className="font-medium">{part.title || part.name}</span>
        {running ? <span className="text-xs text-zinc-500">执行中</span> : null}
      </div>
      {part.summary ? (
        <p className="mt-1 text-xs text-zinc-600">{part.summary}</p>
      ) : null}
    </div>
  );
}
