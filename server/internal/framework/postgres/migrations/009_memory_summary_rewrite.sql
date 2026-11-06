ALTER TABLE memory_jobs DROP CONSTRAINT memory_jobs_kind_check;
ALTER TABLE memory_jobs ADD CONSTRAINT memory_jobs_kind_check CHECK(kind IN ('extract','summarize','maintain','rewrite_summary'));
