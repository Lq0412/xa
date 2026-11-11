import assert from 'node:assert/strict';
import test from 'node:test';
import { applyRunEvent } from '../src/services/chat-run.ts';

test('applyRunEvent 合并工具生命周期', () => {
  let parts = applyRunEvent([], {
    seq: 1,
    type: 'tool.preparing',
    name: 'current_time',
    title: '当前时间',
    index: 0,
  });
  parts = applyRunEvent(parts, {
    seq: 2,
    type: 'tool.started',
    call_id: 'c1',
    name: 'current_time',
    title: '当前时间',
  });
  parts = applyRunEvent(parts, {
    seq: 3,
    type: 'tool.completed',
    call_id: 'c1',
    name: 'current_time',
    status: 'succeeded',
    summary: 'ok',
  });
  const tool = parts.find((p) => p.type === 'tool');
  assert.ok(tool);
  assert.equal(tool.status, 'succeeded');
});

test('按步骤和索引关联同名工具的准备与调用事件', () => {
  const events = [
    { type: 'tool.preparing', step: 1, index: 0 },
    { type: 'tool.preparing', step: 1, index: 1 },
    { type: 'tool.called', step: 1, index: 0, call_id: 'c1' },
    { type: 'tool.called', step: 1, index: 1, call_id: 'c2' },
    { type: 'tool.preparing', step: 2, index: 0 },
    { type: 'tool.called', step: 2, index: 0, call_id: 'c3' },
  ];
  const parts = events.reduce<ReturnType<typeof applyRunEvent>>(
    (current, event, index) =>
      applyRunEvent(current, {
        ...event,
        name: 'current_time',
        seq: index + 1,
      }),
    [],
  );
  assert.deepEqual(
    parts.map((part) => part.type === 'tool' && part.call_id),
    ['c1', 'c2', 'c3'],
  );
});
