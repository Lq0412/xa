CREATE TABLE IF NOT EXISTS conversations (
 id text PRIMARY KEY, workspace_id text NOT NULL REFERENCES workspaces(id), user_id text NOT NULL REFERENCES users(id), agent_id text NOT NULL REFERENCES agent_definitions(id), title text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS messages (
 id text PRIMARY KEY, conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, role text NOT NULL CHECK (role IN ('system','user','assistant')), content text NOT NULL, model text NOT NULL DEFAULT '', prompt_tokens integer NOT NULL DEFAULT 0, completion_tokens integer NOT NULL DEFAULT 0, latency_ms bigint NOT NULL DEFAULT 0, status text NOT NULL CHECK (status IN ('pending','completed','failed')), error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS conversations_owner_idx ON conversations(workspace_id,user_id,updated_at DESC);
CREATE INDEX IF NOT EXISTS messages_conversation_idx ON messages(conversation_id,created_at);
