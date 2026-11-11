import assert from 'node:assert/strict';
import test from 'node:test';
import {
  approvalCopy,
  approvalCountdown,
  approvalReceipt,
  type PendingApproval,
} from '../src/pages/chat/approval-copy.ts';
import {
  isActiveToolStatus,
  pendingApprovals,
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
    { title: '等待你确认：遗忘记忆', phase: 'approval', live: false },
  );
  assert.equal(isActiveToolStatus('awaiting_approval'), false);
  assert.equal(
    resolveTimelineOpen({
      userToggled: false,
      userOpen: false,
      failed: false,
    }),
    false,
  );
  assert.equal(
    resolveTimelineOpen({
      userToggled: true,
      userOpen: false,
      failed: false,
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

const forget: PendingApproval = {
  type: 'tool',
  name: 'memory_forget',
  title: '遗忘记忆',
  status: 'awaiting_approval',
  args_preview: '{"memory_id":"mem_preview"}',
  approval: {
    approval_id: 'first',
    expires_at: '2026-11-10T12:00:00Z',
    summary: '将删除这条记忆',
    items: [
      { label: '内容', value: '我喜欢乌龙茶' },
      { label: '命名空间', value: 'preferences' },
    ],
  },
};

test('确认主句使用内容，详情移除重复内容，通用工具不展示参数', () => {
  assert.equal(
    approvalCopy(forget).sentence,
    '要忘掉「我喜欢乌龙茶」这条记忆。',
  );
  assert.deepEqual(approvalCopy(forget).detailItems, [
    { label: '命名空间', value: 'preferences' },
  ]);
  assert.equal(
    approvalCopy({ ...forget, approval: { ...forget.approval, items: [] } })
      .sentence,
    '将删除这条记忆。',
  );
  assert.equal(
    approvalCopy({
      ...forget,
      name: 'other',
      title: '其他工具',
      approval: undefined,
    }).sentence,
    '使用其他工具。',
  );
});

test('收据保留批准范围，拒绝、超时和取消各有文案', () => {
  for (const [remember, text] of [
    ['once', '已允许本次'],
    ['conversation', '已允许，本对话不再询问遗忘记忆'],
    ['agent', '已允许，以后在这个 Agent 上都不再询问遗忘记忆'],
  ] as const) {
    const [resolved] = applyRunEvent([{ ...forget, call_id: 'call' }], {
      seq: 2,
      type: 'approval.resolved',
      call_id: 'call',
      approval_id: 'first',
      decision: 'approved',
      remember,
    });
    assert.equal(resolved.type === 'tool' && approvalReceipt(resolved), text);
  }
  for (const [decision, text] of [
    ['rejected', '已拒绝忘掉「我喜欢乌龙茶」'],
    ['expired', '超时未确认，已视为拒绝'],
    ['cancelled', '已取消'],
  ] as const)
    assert.equal(
      approvalReceipt({
        ...forget,
        approval: { ...forget.approval, decision },
      }),
      text,
    );
});

test('历史审批没有展示条目时仍能渲染工具收据', () => {
  const tool = {
    ...forget,
    name: 'current_time',
    title: '当前时间',
    approval: {
      ...forget.approval,
      items: null,
      decision: 'approved' as const,
    },
  };
  assert.equal(approvalReceipt(tool), '已允许本次');
  assert.deepEqual(approvalCopy(tool).detailItems, []);
});

test('倒计时不足两分钟才显示，待确认按时间排队', () => {
  const expiresAt = forget.approval.expires_at;
  const expiry = Date.parse(expiresAt);
  assert.equal(approvalCountdown(expiresAt, expiry - 120_000), '');
  assert.equal(
    approvalCountdown(expiresAt, expiry - 80_000),
    '将在 1 分 20 秒后视为拒绝',
  );
  assert.equal(approvalCountdown(expiresAt, expiry), '已到期，等待处理');
  const later = {
    ...forget,
    title: '其他工具',
    approval: {
      ...forget.approval,
      approval_id: 'later',
      expires_at: '2026-11-10T12:00:01Z',
    },
  };
  assert.equal(
    pendingApprovals([later, forget])[0].approval.approval_id,
    'first',
  );
  assert.equal(
    processSummary({
      parts: [later, forget],
      messageStatus: 'pending',
      reasoningMs: {},
    }).title,
    '等待你确认：遗忘记忆（还有 1 个）',
  );
});
