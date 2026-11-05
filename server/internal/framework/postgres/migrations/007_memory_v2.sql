ALTER TABLE memory_items DROP CONSTRAINT IF EXISTS memory_items_status_check;
ALTER TABLE memory_items ADD CONSTRAINT memory_items_status_check CHECK(status IN ('active','superseded','deleted','archived'));
ALTER TABLE memory_items DROP CONSTRAINT IF EXISTS memory_items_source_check;
ALTER TABLE memory_items ADD CONSTRAINT memory_items_source_check CHECK(source IN ('auto','user_edited','chat_extraction','tool','import','delegation'));
ALTER TABLE memory_items ADD COLUMN IF NOT EXISTS retention real NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS memory_active_hnsw ON memory_items USING hnsw(embedding vector_cosine_ops) WHERE status='active';

CREATE TABLE IF NOT EXISTS conversation_summaries (
 id text PRIMARY KEY, conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
 agent_id text NOT NULL REFERENCES agent_definitions(id) ON DELETE CASCADE,
 owner_user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 content text NOT NULL, covers_from_message_id text REFERENCES messages(id) ON DELETE SET NULL,
 covers_until_message_id text REFERENCES messages(id) ON DELETE SET NULL,
 message_count integer NOT NULL, estimated_tokens integer NOT NULL, version integer NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS conversation_summaries_latest ON conversation_summaries(conversation_id,version DESC);

CREATE TABLE IF NOT EXISTS memory_jobs (
 id text PRIMARY KEY, kind text NOT NULL CHECK(kind IN ('extract','summarize','maintain')),
 ref text NOT NULL, owner_user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','completed','failed','dead')),
 attempts integer NOT NULL DEFAULT 0, next_run_at timestamptz NOT NULL DEFAULT now(),
 last_error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 UNIQUE(kind,ref)
);
CREATE INDEX IF NOT EXISTS memory_jobs_pending ON memory_jobs(status,next_run_at,created_at);
INSERT INTO memory_jobs(id,kind,ref,owner_user_id,status,created_at)
 SELECT 'job_'||source_ref,'extract',source_ref,c.user_id,
 CASE WHEN t.status='completed' THEN 'completed' WHEN t.status='failed' THEN 'failed' ELSE 'pending' END,t.created_at
 FROM memory_tasks t JOIN messages m ON m.id=t.source_ref JOIN conversations c ON c.id=m.conversation_id
 ON CONFLICT (kind,ref) DO NOTHING;
