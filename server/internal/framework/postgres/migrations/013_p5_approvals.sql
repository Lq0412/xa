-- 审批断点和授予独立于已冻结的工具快照。
ALTER TABLE run_contexts ADD COLUMN resume_state jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE run_approvals ADD COLUMN remember text NOT NULL DEFAULT ''
    CHECK (remember IN ('','once','conversation','agent'));

ALTER TABLE conversations ADD CONSTRAINT conversations_owner_unique UNIQUE (id, user_id);
CREATE TABLE conversation_tool_grants (
    conversation_id text NOT NULL,
    tool_name text NOT NULL,
    owner_user_id text NOT NULL REFERENCES users(id),
    decision text NOT NULL CHECK (decision = 'allow'),
    source_approval_id text REFERENCES run_approvals(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (conversation_id, tool_name),
    FOREIGN KEY (conversation_id, owner_user_id) REFERENCES conversations(id, user_id) ON DELETE CASCADE
);
