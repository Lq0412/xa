package definition

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/auth"
)

// Library 保存用户添加的公共 Agent；私有 Agent 直接按当前归属读取。
type Library struct{ DB *pgxpool.Pool }

// List 仅返回当前工作空间内可对话的自有 Agent 和已添加公共 Agent。
func (l *Library) List(ctx context.Context, actor auth.Principal) ([]AgentDefinition, error) {
	if actor.UserID == "" || actor.WorkspaceID == "" {
		return nil, auth.ErrUnauthenticated
	}
	rows, err := l.DB.Query(ctx, definitionQuery()+`
		WHERE d.workspace_id=$1 AND d.deleted_at IS NULL AND d.status='active'
		AND ((d.owner_type='user' AND d.owner_id=$2)
		 OR (d.visibility='public' AND (d.created_by=$2 OR EXISTS (
		     SELECT 1 FROM agent_library l WHERE l.agent_id=d.id AND l.user_id=$2))))
		ORDER BY d.is_primary DESC, d.updated_at DESC, d.id`, actor.WorkspaceID, actor.UserID)
	if err != nil {
		return nil, fmt.Errorf("查询用户 Agent 列表: %w", err)
	}
	defer rows.Close()
	result := make([]AgentDefinition, 0)
	for rows.Next() {
		agent, err := scanDefinitionRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, agent)
	}
	return result, rows.Err()
}

// Add 在同一语句校验公开状态和工作空间；重复添加不产生重复记录。
func (l *Library) Add(ctx context.Context, actor auth.Principal, agentID string) error {
	if actor.UserID == "" || actor.WorkspaceID == "" {
		return auth.ErrUnauthenticated
	}
	result, err := l.DB.Exec(ctx, `INSERT INTO agent_library (user_id, agent_id)
		SELECT $1, d.id FROM agent_definitions d
		WHERE d.id=$2 AND d.workspace_id=$3 AND d.visibility='public'
		AND d.status='active' AND d.deleted_at IS NULL
		ON CONFLICT (user_id, agent_id) DO UPDATE SET agent_id=EXCLUDED.agent_id`, actor.UserID, agentID, actor.WorkspaceID)
	if err != nil {
		return fmt.Errorf("添加公共 Agent: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
