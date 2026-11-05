CREATE TABLE agent_library (
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id text NOT NULL REFERENCES agent_definitions(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, agent_id)
);

-- 已经使用过的公共 Agent 保留在用户列表中，历史会话继续可达。
INSERT INTO agent_library (user_id, agent_id)
SELECT DISTINCT c.user_id, c.agent_id
FROM conversations c JOIN agent_definitions d ON d.id = c.agent_id
WHERE d.visibility = 'public'
ON CONFLICT DO NOTHING;
