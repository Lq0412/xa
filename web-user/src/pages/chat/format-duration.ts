/** 思考/工具用时文案（秒向上取整，与 Vercel Reasoning 一致）。 */
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

export type ReasoningVisualStatus = 'streaming' | 'completed' | 'stopped';

export function resolveReasoningStatus(
  status: 'streaming' | 'completed' | 'stopped',
  laterActivity: boolean,
  messageStopped: boolean,
): ReasoningVisualStatus {
  if (messageStopped && status === 'streaming') return 'stopped';
  if (status === 'streaming' && laterActivity) return 'completed';
  return status;
}

export function reasoningTitle(
  visual: ReasoningVisualStatus,
  elapsedSeconds: number,
  durationMs?: number,
): string {
  if (visual === 'streaming') {
    const sec = elapsedSeconds > 0 ? formatThinkingSeconds(elapsedSeconds) : '';
    return sec ? `思考中… ${sec}` : '思考中…';
  }
  if (visual === 'stopped') {
    const dur = formatDurationMs(durationMs);
    return dur ? `思考已停止 · ${dur}` : '思考已停止';
  }
  const dur = formatDurationMs(durationMs);
  if (dur) return `已思考 ${dur}`;
  return '已思考';
}

export function shouldAutoExpand(visual: ReasoningVisualStatus): boolean {
  return visual === 'streaming';
}

export function shouldAutoCollapse(visual: ReasoningVisualStatus): boolean {
  return visual === 'completed' || visual === 'stopped';
}
