import { useEffect, useId, useRef, useState } from 'react';
import { Icon } from '@/components/Icon';
import { Markdown } from '@/components/Markdown';
import type {
  MessagePart,
  ReasoningPart,
  ToolPart,
} from '@/services/chat-parts';
import { approvalReceipt } from './approval-copy';
import {
  formatDurationMs,
  hasFailedTool,
  isActiveToolStatus,
  isFailedToolStatus,
  preferDurationMs,
  processSummary,
  reasoningHasBody,
  reasoningHistoryLabel,
  reasoningPreview,
  resolveTimelineOpen,
  showStreamingPreview,
  toolStatusText,
  visibleTimelineParts,
} from './process-summary';

type MessageStatus = 'pending' | 'completed' | 'failed' | 'stopped';

type Props = {
  parts: MessagePart[];
  messageStatus: MessageStatus;
};

function useClock(active: boolean) {
  const [now, setNow] = useState(() => performance.now());
  useEffect(() => {
    if (!active) return;
    setNow(performance.now());
    const id = window.setInterval(() => setNow(performance.now()), 1000);
    return () => window.clearInterval(id);
  }, [active]);
  return now;
}

/** 本地只记住每个思考段的起点；服务端 duration_ms 到达后不再使用这段计时。 */
function useReasoningDurations(parts: MessagePart[], now: number) {
  const started = useRef(new Map<string, number>());
  const frozen = useRef(new Map<string, number>());
  const durations: Record<string, number | undefined> = {};
  for (const part of parts) {
    if (part.type !== 'reasoning') continue;
    const server = part.duration_ms;
    if (part.status === 'streaming' && !(server != null && server > 0)) {
      if (!started.current.has(part.item_id))
        started.current.set(part.item_id, performance.now());
    }
    let local: number | undefined;
    const start = started.current.get(part.item_id);
    if (start != null) {
      if (part.status === 'streaming') local = Math.max(0, now - start);
      else if (!frozen.current.has(part.item_id))
        frozen.current.set(
          part.item_id,
          Math.max(0, performance.now() - start),
        );
      local =
        part.status === 'streaming' ? local : frozen.current.get(part.item_id);
    }
    durations[part.item_id] = preferDurationMs(server, local);
  }
  return durations;
}

function chipStatus(part: ToolPart): string {
  if (isActiveToolStatus(part.status)) return 'running';
  if (isFailedToolStatus(part.status)) return 'failed';
  return 'done';
}

function ToolTrace({ part }: { part: ToolPart }) {
  const detailsId = useId();
  const failed = isFailedToolStatus(part.status);
  const active = isActiveToolStatus(part.status);
  const [userToggled, setUserToggled] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const receipt = approvalReceipt(part);
  const open = userToggled ? userOpen : failed && !receipt;
  const duration = !active ? formatDurationMs(part.duration_ms) : '';
  const status = toolStatusText(part.status);
  const title = part.title || part.name;
  const errorText = failed ? part.summary || part.error_code || '调用失败' : '';
  const hasDetails =
    part.status !== 'awaiting_approval' &&
    Boolean(
      errorText ||
        part.summary ||
        part.args_preview ||
        (part.items && part.items.length > 0),
    );
  const label = [title, status, duration].filter(Boolean).join('，');
  const body = (
    <span className="flex min-w-0 flex-1 items-center gap-2">
      <ToolMark part={part} />
      <span className="min-w-0 truncate">{title}</span>
      {status && part.status !== 'succeeded' ? (
        <span
          className={`shrink-0 text-xs ${failed ? 'text-rose-600' : 'text-zinc-400'}`}
        >
          {status}
        </span>
      ) : null}
      {duration ? (
        <span className="shrink-0 text-xs text-zinc-400">{duration}</span>
      ) : null}
    </span>
  );
  return (
    <div
      className="process-tool min-w-0"
      data-testid="tool-call-chip"
      data-status={chipStatus(part)}
      data-tool-status={part.status}
    >
      {hasDetails ? (
        <button
          type="button"
          className="flex w-full min-w-0 items-center gap-2 py-1 text-left text-[13px] text-inherit"
          aria-expanded={open}
          aria-controls={detailsId}
          aria-label={label}
          onClick={() => {
            setUserToggled(true);
            setUserOpen(!open);
          }}
        >
          {body}
          <Icon
            name="chevron"
            size={14}
            className={`shrink-0 text-zinc-400 motion-safe:transition-transform ${open ? 'rotate-90' : ''}`}
          />
        </button>
      ) : (
        <div className="flex w-full min-w-0 items-center gap-2 py-1 text-[13px]">
          {body}
        </div>
      )}
      {receipt && (
        <p
          className="pb-1 pl-[22px] text-xs text-zinc-400"
          data-testid="approval-receipt"
        >
          {receipt}
        </p>
      )}
      {open && hasDetails ? (
        <div id={detailsId} className="space-y-1.5 pb-1 pl-6 text-xs">
          {errorText ? (
            <p className="text-rose-600" data-testid="tool-error">
              {errorText}
            </p>
          ) : part.summary ? (
            <p data-testid="tool-summary">{part.summary}</p>
          ) : null}
          {part.args_preview ? (
            <pre className="max-w-full overflow-x-auto whitespace-pre-wrap font-mono text-[11px] text-zinc-500">
              {part.args_preview}
            </pre>
          ) : null}
          {part.items?.length ? (
            <ul className="space-y-0.5">
              {part.items.map((item) => (
                <li key={`${item.label}-${item.value.slice(0, 24)}`}>
                  <span className="text-zinc-400">{item.label}：</span>
                  {item.value}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function ToolMark({ part }: { part: ToolPart }) {
  if (isActiveToolStatus(part.status)) {
    return (
      <span
        className="inline-block size-3 shrink-0 animate-spin rounded-full border border-current border-t-transparent motion-reduce:animate-none"
        aria-hidden
      />
    );
  }
  if (part.status === 'succeeded') {
    return (
      <Icon name="check" size={14} className="shrink-0 text-emerald-600" />
    );
  }
  if (isFailedToolStatus(part.status)) {
    return <Icon name="alert" size={14} className="shrink-0 text-rose-600" />;
  }
  return (
    <span
      className="inline-block size-2 shrink-0 rounded-full bg-zinc-300"
      aria-hidden
    />
  );
}

function ReasoningSegment({
  part,
  durationMs,
  stopped,
}: {
  part: ReasoningPart;
  durationMs: number | undefined;
  stopped: boolean;
}) {
  if (!reasoningHasBody(part)) {
    return (
      <p
        className="py-1 text-[13px] text-zinc-400"
        data-testid="reasoning-segment"
        data-expandable="0"
      >
        {reasoningHistoryLabel(
          durationMs ?? part.duration_ms,
          stopped || part.status === 'stopped',
        )}
      </p>
    );
  }
  return (
    <div
      className="min-w-0 py-1"
      data-testid="reasoning-segment"
      data-expandable="1"
    >
      <div
        className="markdown-reasoning text-[13px] leading-relaxed text-zinc-500"
        data-testid="reasoning-body"
      >
        <Markdown>{part.text}</Markdown>
      </div>
      {part.truncated ? (
        <p className="mt-1 text-xs text-zinc-400">思考在到达上限时被截断</p>
      ) : null}
    </div>
  );
}

/** 一次回复的思考和工具收成回答上方的一行，展开后是细竖线时间线。 */
export function ProcessRail({ parts, messageStatus }: Props) {
  const timelineId = useId();
  const streaming = parts.some(
    (part) =>
      part.type === 'reasoning' &&
      part.status === 'streaming' &&
      !(part.duration_ms != null && part.duration_ms > 0),
  );
  const now = useClock(streaming);
  const reasoningMs = useReasoningDurations(parts, now);
  const summary = processSummary({ parts, messageStatus, reasoningMs });
  const showDetails = parts.some(
    (part) => part.type === 'tool' && part.approval?.decision,
  );
  const failed = hasFailedTool(parts) || messageStatus === 'failed';
  const [userToggled, setUserToggled] = useState(false);
  const [userOpen, setUserOpen] = useState(false);
  const waiting = summary.phase === 'approval';
  const open = resolveTimelineOpen({
    userToggled,
    userOpen,
    failed: !waiting && (failed || showDetails),
  });
  const streamingPart = [...parts]
    .reverse()
    .find(
      (part): part is ReasoningPart =>
        part.type === 'reasoning' &&
        part.status === 'streaming' &&
        reasoningHasBody(part),
    );
  const preview = streamingPart ? reasoningPreview(streamingPart.text) : '';
  const showPreview =
    !waiting &&
    showStreamingPreview({
      userToggled,
      timelineOpen: open,
      streamingText: preview,
    });
  const timeline = visibleTimelineParts(parts);
  if (!summary.title) return null;
  return (
    <div
      className="process-rail min-w-0 text-[13px] text-zinc-500"
      data-testid="process-rail"
      data-phase={summary.phase}
      data-live={summary.live ? '1' : '0'}
      aria-busy={summary.live ? true : undefined}
    >
      <button
        type="button"
        className="inline-flex max-w-full items-center gap-1.5 py-0.5 text-left text-inherit hover:text-zinc-700"
        aria-expanded={open}
        aria-controls={timelineId}
        data-testid="process-summary"
        onClick={() => {
          setUserToggled(true);
          setUserOpen(!open);
        }}
      >
        <Icon name="spark" size={14} className="shrink-0 text-zinc-400" />
        <span
          className={`min-w-0 truncate ${summary.live ? 'process-summary-shimmer' : ''}`}
        >
          {summary.title}
        </span>
        <Icon
          name="chevron"
          size={14}
          className={`shrink-0 text-zinc-400 motion-safe:transition-transform ${open ? 'rotate-90' : ''}`}
        />
      </button>
      {showPreview ? (
        <p
          className="mt-1 line-clamp-3 whitespace-pre-wrap pl-5 text-xs leading-5 text-zinc-400"
          data-testid="process-preview"
        >
          {preview}
        </p>
      ) : null}
      <div
        id={timelineId}
        hidden={!open || timeline.length === 0}
        data-testid="process-timeline"
      >
        {open && timeline.length > 0 ? (
          <div className="process-timeline mt-1 ml-[7px] border-l pl-3">
            {timeline.map((part) => {
              if (part.type === 'reasoning') {
                return (
                  <ReasoningSegment
                    key={`reasoning-${part.item_id}`}
                    part={part}
                    durationMs={reasoningMs[part.item_id]}
                    stopped={messageStatus === 'stopped'}
                  />
                );
              }
              if (part.type === 'tool') {
                return (
                  <ToolTrace
                    key={`tool-${part.call_id ?? part.name}-${part.index ?? 0}`}
                    part={part}
                  />
                );
              }
              return null;
            })}
          </div>
        ) : null}
      </div>
    </div>
  );
}
