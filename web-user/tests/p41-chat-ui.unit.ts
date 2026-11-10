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
  reasoningTitle,
  resolveReasoningStatus,
  shouldAutoCollapse,
} from '../src/pages/chat/format-duration.ts';
import { applyRunEvent } from '../src/services/chat-run.ts';

test('formatThinkingSeconds 向上取整与分界', () => {
  assert.equal(formatThinkingSeconds(0.2), '不到 1 秒');
  assert.equal(formatThinkingSeconds(1.1), '2 秒');
  assert.equal(formatDurationMs(59_000), '59 秒');
  assert.equal(formatDurationMs(60_000), '1 分');
  assert.equal(formatDurationMs(76_000), '1 分 16 秒');
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

test('resolveReasoningStatus 后续事件触发防御性结束', () => {
  assert.equal(resolveReasoningStatus('streaming', true, false), 'completed');
  assert.equal(resolveReasoningStatus('streaming', false, true), 'stopped');
  assert.equal(reasoningTitle('completed', 0, 1500), '已思考 2 秒');
  assert.ok(shouldAutoCollapse('completed'));
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
