import assert from 'node:assert/strict';
import test from 'node:test';
import {
  reasoningHasLaterActivity,
  reorderReasoningBeforeText,
  splitAssistantParts,
} from '../src/pages/chat/assistant-parts.ts';
import {
  formatDurationMs,
  formatThinkingSeconds,
  includeReasoningSegment,
  preferDurationMs,
  processSummary,
  reasoningHasBody,
  reasoningHistoryLabel,
  reasoningPreview,
  resolveTimelineOpen,
  showStreamingPreview,
  visibleTimelineParts,
} from '../src/pages/chat/process-summary.ts';
import type { MessagePart } from '../src/services/chat-parts.ts';
import { applyRunEvent } from '../src/services/chat-run.ts';

const thought = (
  text: string,
  status: 'streaming' | 'completed' | 'stopped' = 'completed',
  duration_ms?: number,
): MessagePart => ({
  type: 'reasoning',
  item_id: 'r1',
  text,
  status,
  duration_ms,
});

const tool = (
  status: 'running' | 'succeeded' | 'failed',
  duration_ms?: number,
): MessagePart => ({
  type: 'tool',
  name: 'current_time',
  title: '当前时间',
  status,
  duration_ms,
  summary: status === 'failed' ? '时区无效' : '2026-11-10 09:00',
  error_code: status === 'failed' ? 'invalid_config' : undefined,
});

test('formatThinkingSeconds 向上取整与分界', () => {
  assert.equal(formatThinkingSeconds(0.2), '不到 1 秒');
  assert.equal(formatThinkingSeconds(1.1), '2 秒');
  assert.equal(formatDurationMs(59_000), '59 秒');
  assert.equal(formatDurationMs(60_000), '1 分');
  assert.equal(formatDurationMs(76_000), '1 分 16 秒');
});

test('摘要行：思考、工具、进行中、停止和失败', () => {
  assert.equal(
    processSummary({
      parts: [thought('想一想', 'completed', 4000)],
      messageStatus: 'completed',
      reasoningMs: { r1: 4000 },
    }).title,
    '已思考 4 秒',
  );
  assert.equal(
    processSummary({
      parts: [thought('想一想', 'completed', 4000), tool('succeeded', 2000)],
      messageStatus: 'completed',
      reasoningMs: { r1: 4000 },
    }).title,
    '思考并调用了 1 个工具 · 6 秒',
  );
  assert.equal(
    processSummary({
      parts: [tool('succeeded', 2000)],
      messageStatus: 'completed',
      reasoningMs: {},
    }).title,
    '调用了 1 个工具 · 2 秒',
  );
  assert.equal(
    processSummary({
      parts: [thought('正在列要点', 'streaming')],
      messageStatus: 'pending',
      reasoningMs: { r1: 3000 },
    }).title,
    '正在思考… 3 秒',
  );
  assert.equal(
    processSummary({
      parts: [thought('想完了', 'completed', 2000), tool('running')],
      messageStatus: 'pending',
      reasoningMs: { r1: 2000 },
    }).title,
    '正在调用当前时间…',
  );
  assert.equal(
    processSummary({
      parts: [thought('停在这里', 'stopped', 4000)],
      messageStatus: 'stopped',
      reasoningMs: { r1: 4000 },
    }).title,
    '已停止 · 4 秒',
  );
  assert.equal(
    processSummary({
      parts: [thought('', 'completed')],
      messageStatus: 'failed',
      reasoningMs: {},
    }).title,
    '生成失败',
  );
});

test('手动折叠不被自动开合覆盖，失败默认展开', () => {
  assert.equal(
    resolveTimelineOpen({ userToggled: false, userOpen: false, failed: false }),
    false,
  );
  assert.equal(
    resolveTimelineOpen({ userToggled: false, userOpen: false, failed: true }),
    true,
  );
  assert.equal(
    resolveTimelineOpen({ userToggled: true, userOpen: true, failed: false }),
    true,
  );
  assert.equal(
    resolveTimelineOpen({ userToggled: true, userOpen: false, failed: true }),
    false,
  );
  assert.equal(
    showStreamingPreview({
      userToggled: false,
      timelineOpen: false,
      streamingText: '最后一行',
    }),
    true,
  );
  assert.equal(
    showStreamingPreview({
      userToggled: true,
      timelineOpen: false,
      streamingText: '最后一行',
    }),
    false,
  );
});

test('计时优先用服务端 duration_ms，缺失时用本地', () => {
  assert.equal(preferDurationMs(4100, 9000), 4100);
  assert.equal(preferDurationMs(undefined, 9000), 9000);
  assert.equal(preferDurationMs(0, 2000), 2000);
  assert.equal(preferDurationMs(Number.NaN, 1500), 1500);
});

test('空思考段不进入时间线，历史占位保留', () => {
  const parts: MessagePart[] = [
    thought('   ', 'streaming'),
    thought('', 'completed', 4000),
    tool('succeeded', 100),
  ];
  parts[1] = { ...thought('', 'completed', 4000), item_id: 'r2' };
  const visible = visibleTimelineParts(parts);
  assert.equal(visible.length, 2);
  assert.equal(visible[0].type, 'reasoning');
  assert.equal(includeReasoningSegment(thought('  ', 'streaming')), false);
  assert.equal(reasoningHasBody(thought('', 'completed', 4000)), false);
  assert.equal(reasoningHistoryLabel(4000, false), '已思考 4 秒');
  assert.equal(reasoningHistoryLabel(undefined, true), '思考已停止');
  assert.equal(
    reasoningPreview('第一行\n第二行\n第三行\n第四行'),
    '第二行\n第三行\n第四行',
  );
});

test('reorderReasoningBeforeText 把思考挪回正文前', () => {
  const ordered = reorderReasoningBeforeText([
    { type: 'text', text: '回答' },
    { type: 'reasoning', item_id: 'r1', text: '', status: 'completed' },
    { type: 'tool', name: 't', title: '工具', status: 'succeeded' },
  ]);
  assert.equal(ordered[0].type, 'reasoning');
  assert.equal(ordered[1].type, 'tool');
  assert.equal(ordered[2].type, 'text');
});

test('splitAssistantParts 分离过程轨与正文', () => {
  const { process, text } = splitAssistantParts([
    { type: 'reasoning', item_id: 'r1', text: 'a', status: 'completed' },
    { type: 'tool', name: 'current_time', title: '时间', status: 'running' },
    { type: 'text', text: '现在是' },
  ]);
  assert.equal(process.length, 2);
  assert.equal(text.length, 1);
});

test('applyRunEvent 写入 duration_ms', () => {
  let parts = applyRunEvent([], {
    seq: 1,
    type: 'reasoning.delta',
    item_id: 'r1',
    delta: '1',
  });
  parts = applyRunEvent(parts, {
    seq: 2,
    type: 'reasoning.completed',
    item_id: 'r1',
    duration_ms: 900,
  });
  const r = parts.find((p) => p.type === 'reasoning');
  assert.ok(r && r.type === 'reasoning');
  assert.equal(r.duration_ms, 900);
  assert.equal(r.status, 'completed');
});

test('reasoningHasLaterActivity', () => {
  const parts = applyRunEvent(
    applyRunEvent([], {
      seq: 1,
      type: 'reasoning.delta',
      item_id: 'r1',
      delta: 'x',
    }),
    {
      seq: 2,
      type: 'tool.preparing',
      name: 'current_time',
      title: '时间',
      index: 0,
    },
  );
  assert.equal(reasoningHasLaterActivity(parts, 'r1'), true);
});
