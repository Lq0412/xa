import type {
  MessagePart,
  ReasoningPart,
  ToolPart,
} from '@/services/chat-parts';

/** 思考和工具用时。不足 1 秒单独写，满 1 分钟后用分和秒。 */
export function formatThinkingSeconds(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds <= 0) return '';
  if (totalSeconds < 1) return '不到 1 秒';
  if (totalSeconds < 60) return `${Math.ceil(totalSeconds)} 秒`;
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = Math.ceil(totalSeconds % 60);
  if (seconds === 0) return `${minutes} 分`;
  return `${minutes} 分 ${seconds} 秒`;
}

export function formatDurationMs(ms?: number): string {
  if (ms == null || !Number.isFinite(ms) || ms <= 0) return '';
  return formatThinkingSeconds(ms / 1000);
}

export type ProcessPhase =
  | 'approval'
  | 'thinking'
  | 'tool'
  | 'settled'
  | 'stopped'
  | 'failed';

export type ProcessSummary = {
  title: string;
  phase: ProcessPhase;
  live: boolean;
};

type MessageStatus = 'pending' | 'completed' | 'failed' | 'stopped';

/** 服务端 duration_ms 可用时优先；缺失或不是正数时才用本地计时。 */
export function preferDurationMs(
  server: number | undefined,
  localMs: number | undefined,
): number | undefined {
  if (server != null && Number.isFinite(server) && server > 0) return server;
  if (localMs != null && Number.isFinite(localMs) && localMs > 0)
    return localMs;
  return undefined;
}

export function isActiveToolStatus(status: ToolPart['status']): boolean {
  return status === 'preparing' || status === 'pending' || status === 'running';
}

export function isFailedToolStatus(status: ToolPart['status']): boolean {
  return status === 'failed' || status === 'unknown' || status === 'rejected';
}

/**
 * 流式但还没有正文的思考段不占时间线。
 * 已结束且正文被历史接口剥掉时保留一行「已思考」，不渲染空框。
 */
export function includeReasoningSegment(part: ReasoningPart): boolean {
  if (part.text.trim().length > 0) return true;
  if (part.status === 'streaming') return false;
  return part.status === 'completed' || part.status === 'stopped';
}

export function reasoningHasBody(part: ReasoningPart): boolean {
  return part.text.trim().length > 0;
}

export function visibleTimelineParts(parts: MessagePart[]): MessagePart[] {
  return parts.filter((part) => {
    if (part.type === 'tool') return true;
    if (part.type === 'reasoning') return includeReasoningSegment(part);
    return false;
  });
}

/** 摘要行下方只露出当前思考的末尾几行，避免整段思维链占满回答。 */
export function reasoningPreview(text: string, maxLines = 3): string {
  const trimmed = text.replace(/\s+$/u, '');
  if (!trimmed) return '';
  const tail = trimmed.split('\n').slice(-maxLines).join('\n').trim();
  if (tail.length <= 240) return tail;
  return tail.slice(tail.length - 240);
}

export function toolStatusText(status: ToolPart['status']): string {
  switch (status) {
    case 'preparing':
      return '准备中';
    case 'pending':
      return '待执行';
    case 'awaiting_approval':
      return '待确认';
    case 'running':
      return '执行中';
    case 'succeeded':
      return '完成';
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

export function hasFailedTool(parts: MessagePart[]): boolean {
  return parts.some(
    (part) => part.type === 'tool' && isFailedToolStatus(part.status),
  );
}

export type FoldInput = {
  userToggled: boolean;
  userOpen: boolean;
  failed: boolean;
  approval?: boolean;
};

/** 手动开合之后不再自动覆盖。未手动操作时，失败展开，其余默认收起。 */
export function resolveTimelineOpen(input: FoldInput): boolean {
  if (input.userToggled) return input.userOpen;
  return input.failed || Boolean(input.approval);
}

export function showStreamingPreview(input: {
  userToggled: boolean;
  timelineOpen: boolean;
  streamingText: string;
}): boolean {
  if (input.userToggled || input.timelineOpen) return false;
  return input.streamingText.trim().length > 0;
}

function durationTotal(
  parts: MessagePart[],
  reasoningMs: Record<string, number | undefined>,
): number | undefined {
  let total = 0;
  let any = false;
  for (const part of parts) {
    if (part.type === 'reasoning') {
      const ms = reasoningMs[part.item_id];
      if (ms != null && ms > 0) {
        total += ms;
        any = true;
      }
      continue;
    }
    if (
      part.type === 'tool' &&
      part.duration_ms != null &&
      part.duration_ms > 0
    ) {
      total += part.duration_ms;
      any = true;
    }
  }
  return any ? total : undefined;
}

function activePart(parts: MessagePart[]): MessagePart | undefined {
  for (let i = parts.length - 1; i >= 0; i--) {
    const part = parts[i];
    if (part.type === 'reasoning' && part.status === 'streaming') return part;
    if (part.type === 'tool' && isActiveToolStatus(part.status)) return part;
  }
  return undefined;
}

/** 一次回复的过程摘要。进行中写当前步骤，结束后收成一行。 */
export function processSummary(input: {
  parts: MessagePart[];
  messageStatus: MessageStatus;
  reasoningMs: Record<string, number | undefined>;
}): ProcessSummary {
  const { parts, messageStatus, reasoningMs } = input;
  const total = durationTotal(parts, reasoningMs);
  const dur = total != null ? formatThinkingSeconds(total / 1000) : '';
  const tools = parts.filter((part): part is ToolPart => part.type === 'tool');
  const anyReasoning = parts.some(
    (part) =>
      part.type === 'reasoning' &&
      (part.status === 'streaming' || includeReasoningSegment(part)),
  );
  if (messageStatus === 'stopped') {
    return {
      title: dur ? `已停止 · ${dur}` : '已停止',
      phase: 'stopped',
      live: false,
    };
  }
  if (messageStatus === 'failed') {
    return {
      title: dur ? `生成失败 · ${dur}` : '生成失败',
      phase: 'failed',
      live: false,
    };
  }
  const active = messageStatus === 'pending' ? activePart(parts) : undefined;
  const awaiting = tools.filter((part) => part.status === 'awaiting_approval');
  if (messageStatus === 'pending' && awaiting.length) {
    return {
      title:
        awaiting.length === 1
          ? `需要确认：${awaiting[0].title || awaiting[0].name}`
          : `需要确认 ${awaiting.length} 个操作`,
      phase: 'approval',
      live: false,
    };
  }
  if (active?.type === 'reasoning') {
    const ms = reasoningMs[active.item_id];
    const sec = ms != null && ms > 0 ? formatThinkingSeconds(ms / 1000) : '';
    return {
      title: sec ? `正在思考… ${sec}` : '正在思考…',
      phase: 'thinking',
      live: true,
    };
  }
  if (active?.type === 'tool') {
    return {
      title: `正在调用${active.title || active.name}…`,
      phase: 'tool',
      live: true,
    };
  }
  if (messageStatus === 'pending') {
    return { title: '正在处理…', phase: 'thinking', live: true };
  }
  if (tools.length > 0 && anyReasoning) {
    return {
      title: dur
        ? `思考并调用了 ${tools.length} 个工具 · ${dur}`
        : `思考并调用了 ${tools.length} 个工具`,
      phase: 'settled',
      live: false,
    };
  }
  if (tools.length > 0) {
    return {
      title: dur
        ? `调用了 ${tools.length} 个工具 · ${dur}`
        : `调用了 ${tools.length} 个工具`,
      phase: 'settled',
      live: false,
    };
  }
  if (anyReasoning) {
    return {
      title: dur ? `已思考 ${dur}` : '已思考',
      phase: 'settled',
      live: false,
    };
  }
  return { title: '', phase: 'settled', live: false };
}

/** 历史思考没有正文，时间线上只留用时，不能再展开出空内容。 */
export function reasoningHistoryLabel(
  durationMs: number | undefined,
  stopped: boolean,
): string {
  const dur = formatDurationMs(durationMs);
  if (stopped) return dur ? `思考已停止 · ${dur}` : '思考已停止';
  return dur ? `已思考 ${dur}` : '已思考';
}
