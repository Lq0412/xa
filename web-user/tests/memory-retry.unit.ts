import assert from 'node:assert/strict';
import { describe, it } from 'node:test';
import {
  composeRetryNote,
  confirmedRetryNote,
  createRetryGate,
  type DeadJob,
  deadJobIDs,
  manualMemoryBody,
  parseDeadJobs,
  runDeadRetry,
  unconfirmedRetryNote,
  withRetryGate,
} from '../src/services/memoryRetry.ts';

function job(id: string): DeadJob {
  return {
    id,
    kind: 'extract',
    last_error_summary: '模型服务请求失败',
    updated_at: '2026-11-05T00:01:00Z',
  };
}

function failed(status: number) {
  return Object.assign(new Error(`请求失败（${status}）`), { status });
}

function snapshot(dead: number, ids: string[]) {
  return { dead, deadJobs: ids.map((id) => job(id)) };
}

describe('parseDeadJobs', () => {
  it('没有任务时是空数组', () => {
    assert.deepEqual(parseDeadJobs([]), []);
  });

  it('读取真实 dead_jobs 字段', () => {
    const jobs = parseDeadJobs([
      job('job_1'),
      {
        id: 'job_2',
        kind: 'maintain',
        last_error_summary: '任务执行失败',
        updated_at: '2026-11-05T00:02:00Z',
      },
    ]);
    assert.deepEqual(
      jobs.map((item) => item.id),
      ['job_1', 'job_2'],
    );
  });

  it('缺少 dead_jobs 或条目不完整时失败', () => {
    assert.throws(() => parseDeadJobs(undefined), /记忆统计格式不正确/);
    assert.throws(() => parseDeadJobs([{ id: 'job_1' }]), /记忆统计格式不正确/);
  });
});

describe('retryDeadBatch', () => {
  it('单任务成功只请求一次，刷新后没有剩余', async () => {
    const calls: string[] = [];
    const outcome = await runDeadRetry({
      stats: snapshot(1, ['job_1']),
      retry: async (id) => {
        calls.push(id);
      },
      refreshStats: async () => snapshot(0, []),
      refreshList: async () => ({ items: 1 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.deepEqual(calls, ['job_1']);
    assert.equal(outcome.note, '已重新排队 1 个');
    assert.equal(outcome.stats?.dead, 0);
    assert.equal(outcome.alert, false);
  });

  it('多个任务全部重新入队', async () => {
    const calls: string[] = [];
    const outcome = await runDeadRetry({
      stats: snapshot(3, ['job_1', 'job_2', 'job_3']),
      retry: async (id) => {
        calls.push(id);
      },
      refreshStats: async () => snapshot(0, []),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.deepEqual(calls, ['job_1', 'job_2', 'job_3']);
    assert.equal(outcome.note, '已重新排队 3 个');
  });

  it('重复 id 只请求一次', async () => {
    const calls: string[] = [];
    await runDeadRetry({
      stats: snapshot(2, ['job_1', 'job_1', 'job_2']),
      retry: async (id) => {
        calls.push(id);
      },
      refreshStats: async () => snapshot(0, []),
      refreshList: async () => ({ items: 0 }),
    });
    assert.deepEqual(calls, ['job_1', 'job_2']);
    assert.deepEqual(
      deadJobIDs(snapshot(2, ['job_1', 'job_1', 'job_2']).deadJobs),
      ['job_1', 'job_2'],
    );
  });

  it('总数超过 20 时只重试本次返回的 id，并用刷新后的 dead 作为剩余', async () => {
    const first = Array.from({ length: 20 }, (_, index) => `job_${index + 1}`);
    const rest = ['job_21', 'job_22', 'job_23', 'job_24', 'job_25'];
    const calls: string[] = [];
    const outcome = await runDeadRetry({
      stats: snapshot(25, first),
      retry: async (id) => {
        calls.push(id);
      },
      refreshStats: async () => snapshot(5, rest),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(calls.length, 20);
    assert.deepEqual(calls, first);
    for (const id of rest) assert.equal(calls.includes(id), false);
    assert.equal(
      outcome.note,
      '已重新排队 20 个，还有 5 个失败任务，可继续重试',
    );
    assert.equal(outcome.stats?.dead, 5);
    assert.equal(outcome.needsReload, false);
  });

  it('部分失败会继续请求其余任务，并停在刷新后的剩余数', async () => {
    const calls: string[] = [];
    const outcome = await runDeadRetry({
      stats: snapshot(3, ['job_1', 'job_2', 'job_3']),
      retry: async (id) => {
        calls.push(id);
        if (id === 'job_2') throw failed(500);
      },
      refreshStats: async () => snapshot(1, ['job_2']),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.deepEqual(calls, ['job_1', 'job_2', 'job_3']);
    assert.deepEqual(outcome.feedback.requeued, ['job_1', 'job_3']);
    assert.deepEqual(outcome.feedback.failed, ['job_2']);
    assert.equal(
      outcome.note,
      '已重新排队 2 个，1 个未能重试，还有 1 个失败任务，可继续重试',
    );
    assert.equal(outcome.alert, true);
  });

  it('全部失败不说已经重新排队', async () => {
    const outcome = await runDeadRetry({
      stats: snapshot(2, ['job_1', 'job_2']),
      retry: async () => {
        throw failed(500);
      },
      refreshStats: async () => snapshot(2, ['job_1', 'job_2']),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(outcome.note, '2 个未能重试，还有 2 个失败任务，可继续重试');
    assert.equal(outcome.note.includes('已重新排队'), false);
  });

  it('404 不当成重新入队，409 记为状态变化', async () => {
    const gone = await runDeadRetry({
      stats: snapshot(1, ['job_1']),
      retry: async () => {
        throw failed(404);
      },
      refreshStats: async () => snapshot(4, ['job_9']),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(gone.ignored, false);
    if (gone.ignored) return;
    assert.deepEqual(gone.feedback.gone, ['job_1']);
    assert.deepEqual(gone.feedback.handledIDs, ['job_1']);
    assert.equal(gone.note, '还有 4 个失败任务，可继续重试');

    const conflict = await runDeadRetry({
      stats: snapshot(1, ['job_2']),
      retry: async () => {
        throw failed(409);
      },
      refreshStats: async () => snapshot(1, ['job_2']),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(conflict.ignored, false);
    if (conflict.ignored) return;
    assert.deepEqual(conflict.feedback.conflict, ['job_2']);
    assert.deepEqual(conflict.feedback.handledIDs, []);
    assert.equal(
      conflict.note,
      '1 个状态已变化，还有 1 个失败任务，可继续重试',
    );
  });

  it('刷新后的剩余数来自新的统计，失败任务留在第一页时不会自动再请求', async () => {
    const calls: string[] = [];
    const same = ['job_1', 'job_2'];
    const outcome = await runDeadRetry({
      stats: snapshot(8, same),
      retry: async (id) => {
        calls.push(id);
        throw failed(500);
      },
      refreshStats: async () => snapshot(8, same),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.deepEqual(calls, same);
    assert.equal(outcome.note, '2 个未能重试，还有 8 个失败任务，可继续重试');
    assert.equal(calls.length, 2);
  });

  it('统计刷新失败不把剩余说成没有', async () => {
    const outcome = await runDeadRetry({
      stats: snapshot(25, ['job_1', 'job_2']),
      retry: async () => undefined,
      refreshStats: async () => {
        throw new Error('无法连接服务，请检查网络后重试');
      },
      refreshList: async () => ({ items: 3 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(outcome.stats, null);
    assert.equal(outcome.needsReload, true);
    assert.equal(
      outcome.note,
      '已重新排队 2 个。统计刷新失败，不能确认剩余失败任务',
    );
    assert.equal(outcome.note.includes('没有剩余'), false);
    assert.deepEqual(outcome.feedback.handledIDs, ['job_1', 'job_2']);
  });

  it('列表刷新失败时仍报告统计里的剩余', async () => {
    const outcome = await runDeadRetry({
      stats: snapshot(4, ['job_1']),
      retry: async () => undefined,
      refreshStats: async () => snapshot(3, ['job_2']),
      refreshList: async () => {
        throw new Error('记忆列表格式不正确');
      },
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(outcome.list, null);
    assert.equal(outcome.stats?.dead, 3);
    assert.equal(
      outcome.note,
      '已重新排队 1 个，还有 3 个失败任务，可继续重试。记忆列表刷新失败',
    );
  });

  it('统计和列表都刷新失败', async () => {
    const outcome = await runDeadRetry({
      stats: snapshot(2, ['job_1']),
      retry: async () => {
        throw failed(500);
      },
      refreshStats: async () => {
        throw new Error('down');
      },
      refreshList: async () => {
        throw new Error('down');
      },
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(
      outcome.note,
      '1 个未能重试。统计刷新失败，不能确认剩余失败任务。记忆列表刷新失败',
    );
  });

  it('全部是 404 且刷新后没有剩余时，不说已经重新排队', async () => {
    const outcome = await runDeadRetry({
      stats: snapshot(1, ['job_1']),
      retry: async () => {
        throw failed(404);
      },
      refreshStats: async () => snapshot(0, []),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(outcome.note, '没有剩余的失败任务');
    assert.equal(outcome.note.includes('已重新排队'), false);
    assert.equal(outcome.note.includes('状态已变化'), false);
  });

  it('dead 大于 0 但没有 id 时，先刷新再重试拿到的 id', async () => {
    const calls: string[] = [];
    let statsReads = 0;
    const outcome = await runDeadRetry({
      stats: snapshot(4, []),
      retry: async (id) => {
        calls.push(id);
      },
      refreshStats: async () => {
        statsReads += 1;
        return statsReads === 1 ? snapshot(4, ['job_9']) : snapshot(0, []);
      },
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.deepEqual(calls, ['job_9']);
    assert.equal(statsReads, 2);
    assert.equal(outcome.note, '已重新排队 1 个');
  });

  it('刷新后仍然没有 id 时不请求重试', async () => {
    let calls = 0;
    let statsReads = 0;
    const outcome = await runDeadRetry({
      stats: snapshot(3, []),
      retry: async () => {
        calls += 1;
      },
      refreshStats: async () => {
        statsReads += 1;
        return snapshot(3, []);
      },
      refreshList: async () => {
        throw new Error('列表不应在没有 id 时刷新');
      },
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(calls, 0);
    assert.equal(statsReads, 1);
    assert.equal(outcome.note, '还有 3 个失败任务，这次没有返回可重试的任务');
    assert.equal(outcome.stats?.dead, 3);
    assert.equal(outcome.alert, true);
  });

  it('没有 id 且统计刷新失败时不重试', async () => {
    let calls = 0;
    const outcome = await runDeadRetry({
      stats: snapshot(2, []),
      retry: async () => {
        calls += 1;
      },
      refreshStats: async () => {
        throw new Error('无法连接服务，请检查网络后重试');
      },
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(calls, 0);
    assert.equal(outcome.stats, null);
    assert.equal(outcome.needsReload, true);
    assert.equal(outcome.note, '统计刷新失败，不能确认可重试的任务');
  });

  it('没有 id 时刷新后 dead 为 0，不发重试', async () => {
    let calls = 0;
    const outcome = await runDeadRetry({
      stats: snapshot(1, []),
      retry: async () => {
        calls += 1;
      },
      refreshStats: async () => snapshot(0, []),
      refreshList: async () => ({ items: 0 }),
    });
    assert.equal(outcome.ignored, false);
    if (outcome.ignored) return;
    assert.equal(calls, 0);
    assert.equal(outcome.note, '没有剩余的失败任务');
    assert.equal(outcome.alert, false);
  });

  it('没有失败任务时不发请求', async () => {
    let calls = 0;
    const outcome = await runDeadRetry({
      stats: snapshot(0, []),
      retry: async () => {
        calls += 1;
      },
      refreshStats: async () => snapshot(0, []),
      refreshList: async () => ({ items: 0 }),
    });
    assert.deepEqual(outcome, { ignored: true });
    assert.equal(calls, 0);
  });
});

describe('重复点击', () => {
  it('进行中的第二轮直接忽略', async () => {
    const gate = createRetryGate();
    let started = 0;
    let release: () => void = () => undefined;
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    const first = withRetryGate(gate, async () => {
      started += 1;
      await blocked;
      return 'done';
    });
    const second = await withRetryGate(gate, async () => {
      started += 1;
      return 'again';
    });
    assert.deepEqual(second, { ignored: true });
    assert.equal(started, 1);
    release();
    assert.equal(await first, 'done');
    const third = await withRetryGate(gate, async () => 'after');
    assert.equal(third, 'after');
  });
});

describe('文案', () => {
  it('成功入队且刷新后清零', () => {
    const feedback = {
      requeued: ['job_1'],
      gone: [],
      conflict: [],
      failed: [],
      handledIDs: ['job_1'],
    };
    assert.equal(confirmedRetryNote(feedback, 0), '已重新排队 1 个');
    const five = {
      ...feedback,
      requeued: ['job_1', 'job_2', 'job_3', 'job_4', 'job_5'],
    };
    assert.equal(confirmedRetryNote(five, 0), '已重新排队 5 个');
    assert.equal(
      unconfirmedRetryNote(feedback),
      '已重新排队 1 个。统计刷新失败，不能确认剩余失败任务',
    );
    assert.equal(
      composeRetryNote(feedback, null, true),
      '已重新排队 1 个。统计刷新失败，不能确认剩余失败任务。记忆列表刷新失败',
    );
  });
});

describe('手动新增', () => {
  it('连续两次不同事实都不带 subject_key', () => {
    const first = manualMemoryBody({
      content: '我住在杭州。',
      namespace: 'profile',
      importance: 5,
      kind: 'fact',
      pinned: false,
      scope: 'user',
    });
    const secondInput = {
      content: '我周末喜欢骑自行车。',
      namespace: 'preferences',
      importance: 6,
      kind: 'fact',
      pinned: false,
      scope: 'user' as const,
      subject_key: 'fixed.fact',
    };
    const second = manualMemoryBody(secondInput);
    assert.equal(Object.hasOwn(first, 'subject_key'), false);
    assert.equal(Object.hasOwn(second, 'subject_key'), false);
    assert.equal(JSON.stringify(first).includes('subject_key'), false);
    assert.equal(JSON.stringify(second).includes('subject_key'), false);
    assert.equal(JSON.stringify(second).includes('fixed.fact'), false);
    assert.notEqual(first.content, second.content);
    assert.equal(first.namespace, 'profile');
    assert.equal(second.namespace, 'preferences');
  });
});
