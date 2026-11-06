-- 历史任务优先使用完成时间；后续所有更新由数据库统一记录，涵盖失败和重试。
ALTER TABLE memory_jobs ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
UPDATE memory_jobs SET updated_at=coalesce(completed_at,created_at);

CREATE FUNCTION memory_job_touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.updated_at = clock_timestamp();
 RETURN NEW;
END;
$$;
CREATE TRIGGER memory_job_updated_at BEFORE UPDATE ON memory_jobs
 FOR EACH ROW EXECUTE FUNCTION memory_job_touch_updated_at();
CREATE INDEX memory_jobs_dead_updated ON memory_jobs(owner_user_id,updated_at DESC,id DESC) WHERE status='dead';
