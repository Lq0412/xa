CREATE TABLE IF NOT EXISTS runs (
  id text PRIMARY KEY,
  workspace_id text NOT NULL REFERENCES workspaces(id),
  owner_user_id text NOT NULL REFERENCES users(id),
  conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  agent_id text NOT NULL REFERENCES agent_definitions(id),
  agent_version_id text NOT NULL REFERENCES agent_versions(id),
  parent_run_id text REFERENCES runs(id) ON DELETE CASCADE,
  root_run_id text NOT NULL,
  parent_tool_call_id text,
  delegation_depth smallint NOT NULL DEFAULT 0 CHECK (delegation_depth BETWEEN 0 AND 3),
  mode text NOT NULL DEFAULT 'interactive' CHECK (mode IN ('interactive','background','delegated')),
  trigger text NOT NULL DEFAULT 'chat' CHECK (trigger IN ('chat','resume','schedule','delegation')),
  status text NOT NULL CHECK (status IN ('queued','running','waiting_approval','completed','failed','cancelled','budget_exhausted','interrupted')),
  idempotency_key text,
  request_hash text,
  user_message_id text NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
  final_message_id text REFERENCES messages(id) ON DELETE SET NULL,
  provider text NOT NULL,
  model text NOT NULL,
  stream_protocol smallint NOT NULL DEFAULT 1,
  client_capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
  limits jsonb NOT NULL,
  toolset jsonb NOT NULL DEFAULT '[]'::jsonb,
  toolset_hash text NOT NULL DEFAULT '',
  steps integer NOT NULL DEFAULT 0,
  model_turns integer NOT NULL DEFAULT 0,
  tool_calls integer NOT NULL DEFAULT 0,
  prompt_tokens integer NOT NULL DEFAULT 0,
  completion_tokens integer NOT NULL DEFAULT 0,
  cached_tokens integer NOT NULL DEFAULT 0,
  ttft_ms integer,
  latency_ms bigint NOT NULL DEFAULT 0,
  active_ms bigint NOT NULL DEFAULT 0,
  error_code text NOT NULL DEFAULT '',
  error text NOT NULL DEFAULT '',
  cancel_reason text NOT NULL DEFAULT '' CHECK (cancel_reason IN ('','user_cancel','client_disconnected','server_shutdown','approval_cancel')),
  anomalies jsonb NOT NULL DEFAULT '[]'::jsonb,
  cancel_requested_at timestamptz,
  last_event_seq bigint NOT NULL DEFAULT 0,
  lease_owner text,
  lease_expires_at timestamptz,
  heartbeat_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  ended_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS runs_one_active_per_conversation ON runs(conversation_id)
  WHERE parent_run_id IS NULL AND status IN ('queued','running','waiting_approval');
CREATE UNIQUE INDEX IF NOT EXISTS runs_idempotency ON runs(conversation_id, idempotency_key)
  WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS runs_owner_created ON runs(owner_user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS runs_conversation_created ON runs(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS runs_agent_created ON runs(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS runs_stale ON runs(lease_expires_at) WHERE status = 'running';
CREATE INDEX IF NOT EXISTS runs_parent ON runs(parent_run_id) WHERE parent_run_id IS NOT NULL;

ALTER TABLE messages ADD COLUMN IF NOT EXISTS run_id text REFERENCES runs(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS messages_run ON messages(run_id) WHERE run_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS run_contexts (
  run_id text PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
  messages jsonb NOT NULL,
  segments jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS run_messages (
  id text PRIMARY KEY,
  run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  step integer NOT NULL,
  ordinal integer NOT NULL,
  role text NOT NULL CHECK (role IN ('assistant','tool')),
  content text NOT NULL DEFAULT '',
  tool_calls jsonb,
  tool_call_id text,
  tool_name text,
  finish_reason text,
  usage jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, ordinal),
  CHECK ((role = 'tool') = (tool_call_id IS NOT NULL)),
  CHECK (role = 'assistant' OR tool_calls IS NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS run_messages_tool_once ON run_messages(run_id, tool_call_id) WHERE role = 'tool';

CREATE TABLE IF NOT EXISTS tool_results (
  id text PRIMARY KEY,
  run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  owner_user_id text NOT NULL,
  step integer NOT NULL,
  call_index integer NOT NULL,
  call_id text NOT NULL,
  tool_name text NOT NULL,
  tool_version text NOT NULL DEFAULT '',
  effect text NOT NULL,
  arguments jsonb,
  raw_arguments text NOT NULL DEFAULT '',
  args_hash text NOT NULL DEFAULT '',
  status text NOT NULL CHECK (status IN ('pending','rejected','awaiting_approval','running','succeeded','failed','cancelled','unknown')),
  error_code text NOT NULL DEFAULT '',
  error_message text NOT NULL DEFAULT '',
  model_content text NOT NULL DEFAULT '',
  data jsonb,
  display jsonb,
  effects jsonb NOT NULL DEFAULT '[]'::jsonb,
  truncated boolean NOT NULL DEFAULT false,
  stored_truncated boolean NOT NULL DEFAULT false,
  total_chars integer NOT NULL DEFAULT 0,
  attempts smallint NOT NULL DEFAULT 0,
  idempotency_key text NOT NULL,
  started_at timestamptz,
  ended_at timestamptz,
  duration_ms integer,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, call_id)
);
CREATE INDEX IF NOT EXISTS tool_results_conversation ON tool_results(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS run_approvals (
  id text PRIMARY KEY,
  run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  owner_user_id text NOT NULL,
  call_id text NOT NULL,
  tool_name text NOT NULL,
  args_hash text NOT NULL,
  display jsonb NOT NULL,
  status text NOT NULL CHECK (status IN ('pending','approved','rejected','expired','cancelled')),
  decided_by text,
  reason text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  decided_at timestamptz,
  UNIQUE (run_id, call_id)
);
CREATE INDEX IF NOT EXISTS run_approvals_pending ON run_approvals(expires_at) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS run_spans (
  id text PRIMARY KEY,
  run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  parent_span_id text,
  kind text NOT NULL CHECK (kind IN ('run','context','context_segment','llm','tool','memory_recall','memory_write','approval','subrun')),
  name text NOT NULL,
  step integer,
  call_id text,
  child_run_id text REFERENCES runs(id) ON DELETE SET NULL,
  status text NOT NULL CHECK (status IN ('ok','error','cancelled')),
  error_code text NOT NULL DEFAULT '',
  attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
  input_preview text NOT NULL DEFAULT '',
  output_preview text NOT NULL DEFAULT '',
  prompt_tokens integer,
  completion_tokens integer,
  cached_tokens integer,
  estimated_tokens integer,
  started_at timestamptz NOT NULL,
  ended_at timestamptz,
  duration_ms integer
);
CREATE INDEX IF NOT EXISTS run_spans_run ON run_spans(run_id, started_at);

CREATE TABLE IF NOT EXISTS run_events (
  run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  seq bigint NOT NULL,
  type text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, seq)
);
