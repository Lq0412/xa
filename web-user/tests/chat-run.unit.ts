import assert from 'node:assert/strict';
import test from 'node:test';
import { applyRunEvent } from '../src/services/chat-run.ts';

test('applyRunEvent 合并工具生命周期', () => {
  let parts = applyRunEvent([], {
    type: 'tool.preparing',
    name: 'current_time',
    title: '当前时间',
    index: 0,
  });
  parts = applyRunEvent(parts, {
    type: 'tool.started',
    call_id: 'c1',
    name: 'current_time',
    title: '当前时间',
  });
  parts = applyRunEvent(parts, {
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
