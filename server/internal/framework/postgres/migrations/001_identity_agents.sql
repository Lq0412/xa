CREATE TABLE IF NOT EXISTS workspaces (
    id text PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id),
    email text NOT NULL,
    password_hash text NOT NULL,
    display_name text NOT NULL,
    role text NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, email)
);

CREATE TABLE IF NOT EXISTS agent_definitions (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id),
    owner_id text NOT NULL,
    owner_type text NOT NULL CHECK (owner_type IN ('user', 'system')),
    current_version_id text,
    name text NOT NULL,
    avatar text NOT NULL DEFAULT '',
    description text NOT NULL DEFAULT '',
    tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    visibility text NOT NULL CHECK (visibility IN ('public', 'private')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'offline', 'deleted')),
    is_default boolean NOT NULL DEFAULT false,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    UNIQUE (workspace_id, name, owner_id)
);

CREATE TABLE IF NOT EXISTS agent_versions (
    id text PRIMARY KEY,
    workspace_id text NOT NULL REFERENCES workspaces(id),
    agent_id text NOT NULL REFERENCES agent_definitions(id),
    version_number integer NOT NULL CHECK (version_number > 0),
    configuration jsonb NOT NULL,
    config_checksum text NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (agent_id, version_number),
    UNIQUE (agent_id, id)
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'agent_definitions_current_version_fk'
    ) THEN
        ALTER TABLE agent_definitions
            ADD CONSTRAINT agent_definitions_current_version_fk
            FOREIGN KEY (current_version_id) REFERENCES agent_versions(id);
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS users_email_idx ON users (workspace_id, lower(email));
CREATE INDEX IF NOT EXISTS agent_definitions_visible_idx
    ON agent_definitions (workspace_id, visibility, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS agent_versions_agent_idx
    ON agent_versions (workspace_id, agent_id, version_number DESC);

INSERT INTO workspaces (id, name)
VALUES ('default', 'Default workspace')
ON CONFLICT (id) DO NOTHING;
