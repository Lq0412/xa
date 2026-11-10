import { useEffect, useId, useState } from 'react';
import { Icon } from '@/components/Icon';
import type { ToolPart } from '@/services/chat-parts';
import { formatDurationMs } from './format-duration';

function statusLabel(part: ToolPart): string {
  switch (part.status) {
    case 'preparing':
      return '准备中';
    case 'pending':
      return '待执行';
    case 'running':
      return '执行中';
    case 'succeeded':
      return '';
    case 'failed':
    case 'unknown':
      return '失败';
    case 'rejected':
      return '未执行';
    case 'cancelled':
      return '已取消';
    default:
      return '';
  }
}

function chipTestStatus(part: ToolPart): string {
  if (
    part.status === 'preparing' ||
    part.status === 'pending' ||
    part.status === 'running'
  )
    return 'running';
  if (part.status === 'succeeded') return 'done';
  if (
    part.status === 'failed' ||
    part.status === 'unknown' ||
    part.status === 'rejected'
  )
    return 'failed';
  return 'done';
}

function defaultOpen(part: ToolPart): boolean {
  return (
    part.status === 'failed' ||
    part.status === 'unknown' ||
    part.status === 'rejected'
  );
}

export function ToolCallChip({ part }: { part: ToolPart }) {
  const detailsId = useId();
  const label = statusLabel(part);
  const running =
    part.status === 'preparing' ||
    part.status === 'pending' ||
    part.status === 'running';
  const ok = part.status === 'succeeded';
  const failed =
    part.status === 'failed' ||
    part.status === 'unknown' ||
    part.status === 'rejected';
  const muted = part.status === 'cancelled' || part.status === 'rejected';
  const [open, setOpen] = useState(defaultOpen(part));
  const [userToggled, setUserToggled] = useState(false);

  useEffect(() => {
    if (userToggled) return;
    if (part.status === 'succeeded' || part.status === 'cancelled')
      setOpen(false);
    if (failed) setOpen(true);
  }, [part.status, failed, userToggled]);

  const duration = formatDurationMs(part.duration_ms);
  const title = part.title || part.name;

  return (
    <div
      className={`rounded-lg border px-3 py-2 text-sm shadow-sm ${muted ? 'border-zinc-200 bg-zinc-50 text-zinc-600' : 'border-zinc-200 bg-white text-zinc-800'}`}
      data-testid="tool-call-chip"
      data-status={chipTestStatus(part)}
      data-tool-status={part.status}
    >
      <button
        type="button"
        className="flex w-full items-center gap-2 text-left"
        onClick={() => {
          setUserToggled(true);
          setOpen((v) => !v);
        }}
        aria-expanded={open}
        aria-controls={detailsId}
      >
        {part.status === 'preparing' ? (
          <span
            className="inline-block h-2 w-2 rounded-full bg-zinc-400 opacity-70"
            aria-hidden
          />
        ) : running ? (
          <span
            className="inline-block h-3 w-3 animate-spin rounded-full border border-zinc-400 border-t-transparent"
            aria-hidden
          />
        ) : ok ? (
          <Icon name="check" className="text-emerald-600" />
        ) : failed ? (
          <Icon name="info" className="text-amber-600" />
        ) : part.status === 'cancelled' ? (
          <span className="h-3 w-3 rounded-full bg-zinc-300" aria-hidden />
        ) : null}
        <span className="min-w-0 flex-1 font-medium">{title}</span>
        {label ? <span className="text-xs text-zinc-500">{label}</span> : null}
        {!running && duration ? (
          <span className="text-xs text-zinc-400">{duration}</span>
        ) : null}
        <span className="text-xs text-zinc-400" aria-hidden>
          {open ? '收起' : '展开'}
        </span>
      </button>
      {!open && part.summary && !running ? (
        <p className="mt-1 line-clamp-2 text-xs text-zinc-600">
          {part.summary}
        </p>
      ) : null}
      <div
        style={{ display: 'grid', gridTemplateRows: open ? '1fr' : '0fr' }}
        className="tool-panel-grid"
      >
        <div id={detailsId} className="overflow-hidden">
          {open ? (
            <div className="mt-2 space-y-2 border-t border-zinc-100 pt-2 text-xs text-zinc-600">
              {part.summary ? (
                <p data-testid="tool-summary">{part.summary}</p>
              ) : null}
              {part.args_preview ? (
                <pre className="whitespace-pre-wrap rounded-md bg-zinc-50 p-2 font-mono text-[11px] text-zinc-700">
                  {part.args_preview}
                </pre>
              ) : null}
              {part.items?.length ? (
                <ul className="space-y-1">
                  {part.items.map((item) => (
                    <li key={`${item.label}-${item.value.slice(0, 24)}`}>
                      <span className="text-zinc-500">{item.label}：</span>
                      {item.value}
                    </li>
                  ))}
                </ul>
              ) : null}
              {part.error_code ? (
                <p className="text-amber-700">{part.error_code}</p>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
