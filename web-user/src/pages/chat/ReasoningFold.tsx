import { useEffect, useId, useRef, useState } from 'react';
import { Markdown } from '@/components/Markdown';
import {
  formatThinkingSeconds,
  reasoningTitle,
  resolveReasoningStatus,
  shouldAutoCollapse,
  shouldAutoExpand,
} from './format-duration';

type Props = {
  itemId: string;
  text: string;
  status: 'streaming' | 'completed' | 'stopped';
  truncated?: boolean;
  duration_ms?: number;
  /** 该思考块之后是否已有工具/正文（防御性结束）。 */
  laterActivity?: boolean;
  messageStopped?: boolean;
};

export function ReasoningFold({
  itemId,
  text,
  status,
  truncated,
  duration_ms,
  laterActivity,
  messageStopped,
}: Props) {
  const bodyId = useId();
  const visual = resolveReasoningStatus(
    status,
    Boolean(laterActivity),
    Boolean(messageStopped),
  );
  const [open, setOpen] = useState(shouldAutoExpand(visual));
  const [userToggled, setUserToggled] = useState(false);
  const [elapsed, setElapsed] = useState(0);
  const startedAt = useRef<number | null>(null);
  const hasBody = text.trim().length > 0;
  const title = reasoningTitle(visual, elapsed, duration_ms);

  useEffect(() => {
    if (visual !== 'streaming') {
      startedAt.current = null;
      return;
    }
    if (startedAt.current == null) startedAt.current = performance.now();
    const tick = () => {
      if (startedAt.current == null) return;
      const sec = (performance.now() - startedAt.current) / 1000;
      setElapsed(Math.max(0, sec));
    };
    tick();
    const id = window.setInterval(tick, 1000);
    return () => window.clearInterval(id);
  }, [visual]);

  useEffect(() => {
    if (userToggled) return;
    if (shouldAutoExpand(visual)) setOpen(true);
  }, [visual, userToggled]);

  useEffect(() => {
    if (userToggled) return;
    if (!shouldAutoCollapse(visual)) return;
    const timer = window.setTimeout(() => setOpen(false), 400);
    return () => window.clearTimeout(timer);
  }, [visual, userToggled]);

  const toggle = () => {
    setUserToggled(true);
    setOpen((v) => !v);
  };

  const showPlaceholder =
    open && !hasBody && visual !== 'streaming' && status !== 'streaming';

  return (
    <div
      className="text-sm text-zinc-600"
      data-testid="reasoning-fold"
      data-reasoning-status={visual}
      data-item-id={itemId}
    >
      <button
        type="button"
        className={`flex w-full items-center justify-between gap-2 py-1.5 text-left text-zinc-700 transition-colors hover:text-zinc-900 ${visual === 'streaming' ? 'reasoning-title-streaming' : ''}`}
        onClick={toggle}
        aria-expanded={open}
        aria-controls={bodyId}
      >
        <span className="min-w-0 truncate">
          {visual === 'streaming' && elapsed > 0
            ? `思考中… ${formatThinkingSeconds(elapsed)}`
            : title}
        </span>
        <span className="shrink-0 text-xs text-zinc-400" aria-hidden="true">
          {open ? '收起' : '展开'}
        </span>
      </button>
      <div
        className="reasoning-panel-grid"
        data-open={open ? '1' : '0'}
        style={{ display: 'grid', gridTemplateRows: open ? '1fr' : '0fr' }}
      >
        <div className="overflow-hidden">
          {open && hasBody ? (
            <div
              id={bodyId}
              className="reasoning-fold-body max-h-[min(40vh,280px)] overflow-y-auto border-l-2 border-zinc-200 py-2 pl-3 text-[13px] leading-relaxed text-zinc-500"
              data-testid="reasoning-fold-body"
              aria-busy={visual === 'streaming' ? true : undefined}
            >
              <div className="markdown-reasoning">
                <Markdown>{text}</Markdown>
              </div>
              {truncated ? (
                <p className="mt-2 text-xs text-zinc-400">
                  思考在到达上限时被截断
                </p>
              ) : null}
            </div>
          ) : null}
          {showPlaceholder ? (
            <p
              id={bodyId}
              className="border-l-2 border-zinc-200 py-2 pl-3 text-xs text-zinc-400"
              data-testid="reasoning-fold-placeholder"
            >
              思考内容未保留在本次加载中
            </p>
          ) : null}
        </div>
      </div>
    </div>
  );
}
