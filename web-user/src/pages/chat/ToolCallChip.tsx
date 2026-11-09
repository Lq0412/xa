import { useEffect, useRef, useState } from 'react';
import { Icon } from '@/components/Icon';
import type { ToolPart } from '@/services/chat-parts';

const MIN_RUNNING_MS = 450;

function isRunning(status: ToolPart['status']) {
  return status === 'preparing' || status === 'pending' || status === 'running';
}

export function ToolCallChip({ part }: { part: ToolPart }) {
  const [shown, setShown] = useState<ToolPart['status']>(() =>
    part.status === 'succeeded' ? 'running' : part.status,
  );
  const hold = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (hold.current) {
      clearTimeout(hold.current);
      hold.current = null;
    }
    if (isRunning(part.status)) {
      setShown(part.status);
      return undefined;
    }
    if (part.status === 'succeeded') {
      if (isRunning(shown)) {
        hold.current = setTimeout(() => setShown('succeeded'), MIN_RUNNING_MS);
        return () => {
          if (hold.current) clearTimeout(hold.current);
        };
      }
      if (shown !== 'succeeded') {
        setShown('running');
        hold.current = setTimeout(() => setShown('succeeded'), MIN_RUNNING_MS);
        return () => {
          if (hold.current) clearTimeout(hold.current);
        };
      }
      return undefined;
    }
    setShown(part.status);
    return undefined;
  }, [part.status, shown]);

  const running = isRunning(shown);
  const ok = shown === 'succeeded';
  const failed = shown === 'failed' || shown === 'unknown';
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
      {part.summary && !running ? (
        <p className="mt-1 text-xs text-zinc-600">{part.summary}</p>
      ) : null}
    </div>
  );
}
