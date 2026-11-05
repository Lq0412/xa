-- 条目与事件由 Memory Service 同事务维护；任务以来源消息保证幂等。
CREATE TABLE memory_items (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES workspaces(id),
 owner_user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 scope text NOT NULL CHECK(scope IN ('user','agent','session')),
 agent_id text REFERENCES agent_definitions(id) ON DELETE CASCADE,
 conversation_id text REFERENCES conversations(id) ON DELETE CASCADE,
 namespace text NOT NULL, subject_key text NOT NULL, kind text NOT NULL DEFAULT 'fact',
 content text NOT NULL, importance integer NOT NULL CHECK(importance BETWEEN 1 AND 10),
 pinned boolean NOT NULL DEFAULT false, confidence real NOT NULL DEFAULT 1,
 sensitive boolean NOT NULL DEFAULT false, strength_days real NOT NULL DEFAULT 60,
 status text NOT NULL CHECK(status IN ('active','superseded','deleted')),
 source text NOT NULL CHECK(source IN ('auto','user_edited')),
 source_ref text REFERENCES messages(id) ON DELETE SET NULL,
 supersedes_id text REFERENCES memory_items(id), embedding vector(1024) NOT NULL,
 valid_from timestamptz NOT NULL DEFAULT now(), valid_to timestamptz, expires_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 last_used_at timestamptz, version integer NOT NULL DEFAULT 1,
 CHECK ((scope='user' AND agent_id IS NULL AND conversation_id IS NULL)
 OR (scope='agent' AND agent_id IS NOT NULL AND conversation_id IS NULL)
 OR (scope='session' AND agent_id IS NOT NULL AND conversation_id IS NOT NULL))
);
CREATE INDEX memory_owner_scope ON memory_items(workspace_id,owner_user_id,scope,agent_id,status,namespace);
CREATE UNIQUE INDEX memory_active_subject ON memory_items(owner_user_id,scope,COALESCE(agent_id,''),COALESCE(conversation_id,''),namespace,subject_key) WHERE status='active';
CREATE INDEX memory_embedding_hnsw ON memory_items USING hnsw(embedding vector_cosine_ops) WHERE status='active';
CREATE INDEX memory_supersedes ON memory_items(supersedes_id);
CREATE TABLE memory_events (
 id bigserial PRIMARY KEY, workspace_id text NOT NULL REFERENCES workspaces(id),
 owner_user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 memory_id text REFERENCES memory_items(id) ON DELETE CASCADE,
 source_ref text REFERENCES messages(id) ON DELETE SET NULL,
 op text NOT NULL, actor_type text NOT NULL, actor_id text NOT NULL,
 before jsonb, after jsonb, reason text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX memory_events_item ON memory_events(owner_user_id,memory_id,id);
CREATE TABLE memory_tasks (
 source_ref text PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','completed','failed','skipped')),
 created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz
);
CREATE INDEX memory_tasks_pending ON memory_tasks(created_at,source_ref) WHERE status='pending';
