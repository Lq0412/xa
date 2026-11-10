import assert from 'node:assert/strict';
import test from 'node:test';
import {
  isActiveToolStatus,
  processSummary,
  resolveTimelineOpen,
} from '../src/pages/chat/process-summary.ts';
import { applyRunEvent } from '../src/services/chat-run.ts';

test('审批按 call_id 归约，等待摘要不转圈，历史决定保留', () => {
  let parts = applyRunEvent([], {
    seq: 1,
    type: 'tool.called',
    call_id: 'first',
    name: 'memory_forget',
    title: '遗忘记忆',
  });
  parts = applyRunEvent(parts, {
    seq: 2,
    type: 'tool.called',
    call_id: 'second',
    name: 'memory_forget',
    title: '遗忘记忆',
  });
  parts = applyRunEvent(parts, {
    seq: 3,
    type: 'approval.required',
    call_id: 'first',
    name: 'memory_forget',
    title: '遗忘记忆',
    approval_id: 'approval',
    expires_at: '2026-11-10T12:00:00Z',
    summary: '将删除这条记忆',
  });
  assert.equal(parts.length, 2);
  assert.deepEqual(
    processSummary({ parts, messageStatus: 'pending', reasoningMs: {} }),
    { title: '需要确认：遗忘记忆', phase: 'approval', live: false },
  );
  assert.equal(isActiveToolStatus('awaiting_approval'), false);
  assert.equal(
    resolveTimelineOpen({
      userToggled: false,
      userOpen: false,
      failed: false,
      approval: true,
    }),
    true,
  );
  assert.equal(
    resolveTimelineOpen({
      userToggled: true,
      userOpen: false,
      failed: false,
      approval: true,
    }),
    false,
  );
  parts = applyRunEvent(parts, {
    seq: 4,
    type: 'approval.resolved',
    call_id: 'first',
    decision: 'expired',
    approval_id: 'approval',
  });
  assert.equal(
    parts[0].type === 'tool' && parts[0].approval?.decision,
    'expired',
  );
  assert.equal(parts[0].type === 'tool' && parts[0].status, 'rejected');
});
