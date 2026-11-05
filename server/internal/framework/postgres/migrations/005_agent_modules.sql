-- 主 Agent 由用户插入触发器创建，注册、管理员初始化与补建共用同一事务边界。
ALTER TABLE agent_definitions ADD COLUMN IF NOT EXISTS owner_user_id text REFERENCES users(id);
ALTER TABLE agent_definitions ADD COLUMN IF NOT EXISTS is_primary boolean NOT NULL DEFAULT false;

UPDATE agent_definitions d SET owner_user_id = owner_id
WHERE owner_type = 'user' AND owner_user_id IS NULL AND EXISTS (SELECT 1 FROM users u WHERE u.id=d.owner_id);

-- 旧的提示词顺序和启用状态保留在 SOUL 内容中；新配置不再存储 prompts。
WITH converted AS (
    SELECT id, (configuration - 'prompts' - 'Prompts') || jsonb_build_object(
        'model', COALESCE(NULLIF(configuration->>'model', ''), 'qwen3.7-plus'),
        'soul_md', COALESCE(configuration->>'soul_md', (
            SELECT string_agg(COALESCE(item->>'content', item->>'Content'), E'\n\n' ORDER BY COALESCE(item->>'order', item->>'Order', '0')::integer, ordinal)
            FROM jsonb_array_elements(CASE
                WHEN jsonb_typeof(configuration->'prompts')='array' THEN configuration->'prompts'
                WHEN jsonb_typeof(configuration->'Prompts')='array' THEN configuration->'Prompts'
                ELSE '[]'::jsonb END) WITH ORDINALITY AS parts(item, ordinal)
            WHERE COALESCE(item->>'enabled', item->>'Enabled', 'false')::boolean
        ), '')
    ) AS config FROM agent_versions
)
UPDATE agent_versions v SET configuration=c.config, config_checksum=encode(sha256(convert_to(c.config::text, 'UTF8')), 'hex')
FROM converted c WHERE v.id=c.id;

-- 若旧数据曾标记多个默认 Agent，只提升最早的一个。
WITH candidates AS (
    SELECT id, row_number() OVER (PARTITION BY owner_user_id ORDER BY created_at, id) AS position
    FROM agent_definitions WHERE owner_user_id IS NOT NULL AND is_default AND deleted_at IS NULL
)
UPDATE agent_definitions d SET is_primary=true FROM candidates c
WHERE d.id=c.id AND c.position=1
AND NOT EXISTS (SELECT 1 FROM agent_definitions p WHERE p.owner_user_id=d.owner_user_id AND p.is_primary AND p.deleted_at IS NULL);

CREATE UNIQUE INDEX IF NOT EXISTS agent_definitions_primary_owner_idx
ON agent_definitions (owner_user_id) WHERE is_primary AND deleted_at IS NULL;

CREATE OR REPLACE FUNCTION create_primary_agent(target_user text, target_workspace text) RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    agent_id text := 'agt_' || gen_random_uuid()::text;
    version_id text := 'agtv_' || gen_random_uuid()::text;
    agent_name text := '主 Agent';
    config jsonb := jsonb_build_object('soul_md', '你是一个简洁、友好、可靠的通用助手。先理解用户，再给出清晰、可执行的回答。', 'model', 'qwen3.7-plus');
BEGIN
    IF EXISTS (SELECT 1 FROM agent_definitions WHERE owner_user_id=target_user AND is_primary AND deleted_at IS NULL) THEN RETURN; END IF;
    IF EXISTS (SELECT 1 FROM agent_definitions WHERE workspace_id=target_workspace AND owner_id=target_user AND name=agent_name) THEN
        agent_name := '主 Agent ' || right(agent_id, 6);
    END IF;
    INSERT INTO agent_definitions (id, workspace_id, owner_id, owner_user_id, owner_type, name, visibility, is_default, is_primary, created_by)
    VALUES (agent_id, target_workspace, target_user, target_user, 'user', agent_name, 'private', true, true, target_user);
    INSERT INTO agent_versions (id, workspace_id, agent_id, version_number, configuration, config_checksum, created_by)
    VALUES (version_id, target_workspace, agent_id, 1, config, encode(sha256(convert_to(config::text, 'UTF8')), 'hex'), target_user);
    UPDATE agent_definitions SET current_version_id=version_id WHERE id=agent_id;
END $$;

CREATE OR REPLACE FUNCTION user_primary_agent_insert() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    PERFORM create_primary_agent(NEW.id, NEW.workspace_id);
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS users_create_primary_agent ON users;
CREATE TRIGGER users_create_primary_agent AFTER INSERT ON users FOR EACH ROW EXECUTE FUNCTION user_primary_agent_insert();

SELECT create_primary_agent(id, workspace_id) FROM users;

-- 同一 Agent 的旧会话先搬移消息再合并，历史内容不会被级联删除。
WITH mapped AS (
    SELECT id, first_value(id) OVER (PARTITION BY workspace_id,user_id,agent_id ORDER BY created_at,id) AS canonical FROM conversations
)
UPDATE messages m SET conversation_id=x.canonical FROM mapped x WHERE m.conversation_id=x.id AND x.id<>x.canonical;

WITH mapped AS (
    SELECT id, first_value(id) OVER (PARTITION BY workspace_id,user_id,agent_id ORDER BY created_at,id) AS canonical FROM conversations
)
DELETE FROM conversations c USING mapped x WHERE c.id=x.id AND x.id<>x.canonical;

CREATE UNIQUE INDEX IF NOT EXISTS conversations_unique_agent_per_user ON conversations(workspace_id,user_id,agent_id);
CREATE INDEX IF NOT EXISTS messages_history_cursor_idx ON messages(conversation_id,created_at DESC,id DESC);

-- 在事务结束时要求现存用户恰好拥有一个主 Agent；删除整个账号的事务仍可正常清理资源。
CREATE OR REPLACE FUNCTION require_primary_agent() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    target_user text;
BEGIN
    IF TG_TABLE_NAME='users' THEN target_user := NEW.id;
    ELSE target_user := OLD.owner_user_id;
    END IF;
    IF target_user IS NOT NULL AND EXISTS (SELECT 1 FROM users WHERE id=target_user)
       AND (SELECT count(*) FROM agent_definitions WHERE owner_user_id=target_user AND is_primary AND deleted_at IS NULL) <> 1 THEN
        RAISE EXCEPTION '用户必须保留一个主 Agent' USING ERRCODE='23514';
    END IF;
    RETURN NULL;
END $$;
DROP TRIGGER IF EXISTS users_require_primary_agent ON users;
CREATE CONSTRAINT TRIGGER users_require_primary_agent AFTER INSERT ON users DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_primary_agent();
DROP TRIGGER IF EXISTS agents_require_primary_agent ON agent_definitions;
CREATE CONSTRAINT TRIGGER agents_require_primary_agent AFTER DELETE OR UPDATE ON agent_definitions DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION require_primary_agent();

ALTER TABLE agent_definitions DROP CONSTRAINT IF EXISTS primary_agent_owner;
ALTER TABLE agent_definitions ADD CONSTRAINT primary_agent_owner CHECK (NOT is_primary OR (owner_user_id IS NOT NULL AND owner_id=owner_user_id AND owner_type='user' AND visibility='private'));

ALTER TABLE messages ADD COLUMN IF NOT EXISTS context_segments jsonb NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS agent_version_id text REFERENCES agent_versions(id);
