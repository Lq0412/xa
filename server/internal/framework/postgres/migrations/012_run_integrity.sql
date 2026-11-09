-- 只添加并验证约束；存量非法数据导致整个迁移事务失败，不修改用户记录。
ALTER TABLE conversations ADD CONSTRAINT conversations_identity_unique UNIQUE (id, workspace_id, user_id);
ALTER TABLE messages ADD CONSTRAINT messages_conversation_unique UNIQUE (id, conversation_id);

ALTER TABLE runs
  ADD COLUMN top_level_id text GENERATED ALWAYS AS (CASE WHEN parent_run_id IS NULL THEN id END) STORED,
  ADD CONSTRAINT runs_top_level_unique UNIQUE (top_level_id),
  ADD CONSTRAINT runs_identity_unique UNIQUE (id, conversation_id, owner_user_id),
  ADD CONSTRAINT runs_owner_unique UNIQUE (id, owner_user_id),
  ADD CONSTRAINT runs_tree_unique UNIQUE (id, root_run_id, conversation_id, owner_user_id),
  ADD CONSTRAINT runs_root_shape CHECK ((parent_run_id IS NULL AND root_run_id=id) OR (parent_run_id IS NOT NULL AND root_run_id<>id AND parent_run_id<>id)),
  ADD CONSTRAINT runs_idempotency_nonempty CHECK (idempotency_key IS NULL OR idempotency_key<>''),
  ADD CONSTRAINT runs_conversation_owner_fk FOREIGN KEY (conversation_id, workspace_id, owner_user_id) REFERENCES conversations(id, workspace_id, user_id) ON DELETE CASCADE,
  ADD CONSTRAINT runs_version_agent_fk FOREIGN KEY (agent_id, agent_version_id) REFERENCES agent_versions(agent_id, id),
  ADD CONSTRAINT runs_user_message_conversation_fk FOREIGN KEY (user_message_id, conversation_id) REFERENCES messages(id, conversation_id) ON DELETE CASCADE,
  ADD CONSTRAINT runs_final_message_conversation_fk FOREIGN KEY (final_message_id, conversation_id) REFERENCES messages(id, conversation_id) ON DELETE SET NULL (final_message_id),
  ADD CONSTRAINT runs_root_fk FOREIGN KEY (root_run_id) REFERENCES runs(top_level_id) ON DELETE CASCADE,
  ADD CONSTRAINT runs_parent_tree_fk FOREIGN KEY (parent_run_id, root_run_id, conversation_id, owner_user_id) REFERENCES runs(id, root_run_id, conversation_id, owner_user_id) ON DELETE CASCADE;

ALTER TABLE tool_results
  ADD CONSTRAINT tool_results_owner_fk FOREIGN KEY (owner_user_id) REFERENCES users(id),
  ADD CONSTRAINT tool_results_run_identity_fk FOREIGN KEY (run_id, conversation_id, owner_user_id) REFERENCES runs(id, conversation_id, owner_user_id) ON DELETE CASCADE,
  ADD CONSTRAINT tool_results_effect_valid CHECK (effect IN ('read','write','destructive')),
  ADD CONSTRAINT tool_results_idempotency_nonempty CHECK (idempotency_key<>'');

ALTER TABLE run_approvals
  ADD CONSTRAINT run_approvals_owner_fk FOREIGN KEY (owner_user_id) REFERENCES users(id),
  ADD CONSTRAINT run_approvals_run_owner_fk FOREIGN KEY (run_id, owner_user_id) REFERENCES runs(id, owner_user_id) ON DELETE CASCADE,
  ADD CONSTRAINT run_approvals_decider_fk FOREIGN KEY (decided_by) REFERENCES users(id),
  ADD CONSTRAINT run_approvals_decider_owner CHECK (decided_by IS NULL OR decided_by=owner_user_id);

CREATE INDEX runs_user_message_idx ON runs(user_message_id);
CREATE INDEX runs_final_message_idx ON runs(final_message_id) WHERE final_message_id IS NOT NULL;
CREATE INDEX runs_agent_version_idx ON runs(agent_version_id);
CREATE INDEX runs_root_idx ON runs(root_run_id);
CREATE INDEX runs_workspace_idx ON runs(workspace_id);
CREATE INDEX tool_results_owner_idx ON tool_results(owner_user_id);
CREATE INDEX run_approvals_owner_idx ON run_approvals(owner_user_id);
CREATE INDEX run_approvals_decider_idx ON run_approvals(decided_by) WHERE decided_by IS NOT NULL;
CREATE INDEX run_spans_child_idx ON run_spans(child_run_id) WHERE child_run_id IS NOT NULL;
