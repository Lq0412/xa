-- 巩固的多个父版本独立记录，supersedes_id 继续表示主要父版本。
CREATE TABLE memory_merge_sources (
 memory_id text NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
 source_id text NOT NULL REFERENCES memory_items(id) ON DELETE CASCADE,
 PRIMARY KEY(memory_id,source_id)
);
CREATE UNIQUE INDEX conversation_summary_version ON conversation_summaries(conversation_id,version);
DROP INDEX IF EXISTS memory_active_hnsw;
-- 旧失败任务进入可重试队列，旧 skipped 任务不会再次执行。
UPDATE memory_jobs j SET status='completed',completed_at=coalesce(j.completed_at,now())
FROM memory_tasks t WHERE j.kind='extract' AND j.ref=t.source_ref AND t.status='skipped';
UPDATE memory_jobs SET status='pending',next_run_at=now() WHERE status='failed';
