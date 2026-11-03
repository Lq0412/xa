package definition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/auth"
)

// PGRepository 将可见性、归属和版本指针检查放进 SQL 与事务，避免只依赖调用方判断。
type PGRepository struct{ pool *pgxpool.Pool }

func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

func (r *PGRepository) Find(ctx context.Context, actor auth.Principal, agentID string) (AgentDefinition, error) {
	return r.scanDefinition(ctx, r.pool.QueryRow(ctx, definitionQuery()+`
		WHERE d.id = $2 AND d.workspace_id = $1 AND d.deleted_at IS NULL
		AND (d.visibility = 'public' OR (d.owner_type = 'user' AND d.owner_id = $3) OR (d.owner_type = 'system' AND $4))`,
		actor.WorkspaceID, agentID, actor.UserID, auth.HasRole(actor, "admin")))
}

func (r *PGRepository) List(ctx context.Context, actor auth.Principal) ([]AgentDefinition, error) {
	rows, err := r.pool.Query(ctx, definitionQuery()+`
		WHERE d.workspace_id = $1 AND d.deleted_at IS NULL
		AND (d.visibility = 'public' OR (d.owner_type = 'user' AND d.owner_id = $2) OR (d.owner_type = 'system' AND $3))
		ORDER BY d.updated_at DESC`, actor.WorkspaceID, actor.UserID, auth.HasRole(actor, "admin"))
	if err != nil {
		return nil, fmt.Errorf("查询 Agent 列表: %w", err)
	}
	defer rows.Close()
	var definitions []AgentDefinition
	for rows.Next() {
		definition, err := scanDefinitionRow(rows)
		if err != nil {
			return nil, err
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取 Agent 列表: %w", err)
	}
	return definitions, nil
}

func (r *PGRepository) FindVersion(ctx context.Context, actor auth.Principal, agentID, versionID string) (AgentVersion, error) {
	var version AgentVersion
	var configuration []byte
	err := r.pool.QueryRow(ctx, `
		SELECT v.id, v.workspace_id, v.agent_id, v.version_number, v.configuration,
		       v.config_checksum, v.created_by, v.created_at
		FROM agent_versions v JOIN agent_definitions d ON d.id = v.agent_id
		WHERE v.workspace_id = $1 AND v.agent_id = $2 AND v.id = $3 AND d.deleted_at IS NULL
		AND (d.visibility = 'public' OR (d.owner_type = 'user' AND d.owner_id = $4) OR (d.owner_type = 'system' AND $5))`,
		actor.WorkspaceID, agentID, versionID, actor.UserID, auth.HasRole(actor, "admin")).
		Scan(&version.ID, &version.WorkspaceID, &version.AgentID, &version.VersionNumber, &configuration,
			&version.ConfigChecksum, &version.CreatedBy, &version.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentVersion{}, ErrNotFound
	}
	if err != nil {
		return AgentVersion{}, fmt.Errorf("查询 Agent 版本: %w", err)
	}
	if err := json.Unmarshal(configuration, &version.Configuration); err != nil {
		return AgentVersion{}, fmt.Errorf("解析 Agent 配置: %w", err)
	}
	return version, nil
}

func (r *PGRepository) SaveRevision(ctx context.Context, actor auth.Principal, saved Saved, expectedVersionID string) error {
	if err := checkManageForSave(actor, saved.Definition); err != nil {
		return err
	}
	configuration, err := json.Marshal(saved.Version.Configuration)
	if err != nil {
		return fmt.Errorf("编码 Agent 配置: %w", err)
	}
	tags, err := json.Marshal(saved.Definition.Tags)
	if err != nil {
		return fmt.Errorf("编码 Agent 标签: %w", err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("开始 Agent 配置事务: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if saved.Definition.CurrentVersionID == "" {
		_, err = tx.Exec(ctx, `
			INSERT INTO agent_definitions
			(id, workspace_id, owner_id, owner_type, name, avatar, description, tags, visibility, status, is_default, created_by, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			saved.Definition.ID, saved.Definition.WorkspaceID, saved.Definition.OwnerID, saved.Definition.OwnerType,
			saved.Definition.Name, saved.Definition.Avatar, saved.Definition.Description, tags,
			saved.Definition.Visibility, saved.Definition.Status, saved.Definition.IsDefault, saved.Definition.CreatedBy,
			saved.Definition.CreatedAt, saved.Definition.UpdatedAt)
		if err != nil {
			if isUniqueViolation(err) {
				return ErrConflict
			}
			return fmt.Errorf("创建 Agent: %w", err)
		}
	} else {
		var currentVersionID string
		err = tx.QueryRow(ctx, `SELECT current_version_id FROM agent_definitions WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL FOR UPDATE`, saved.Definition.ID, actor.WorkspaceID).Scan(&currentVersionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("锁定 Agent: %w", err)
		}
		if currentVersionID != expectedVersionID {
			return ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE agent_definitions SET name=$1, avatar=$2, description=$3, tags=$4, visibility=$5, updated_at=$6 WHERE id=$7`,
			saved.Definition.Name, saved.Definition.Avatar, saved.Definition.Description, tags, saved.Definition.Visibility, saved.Definition.UpdatedAt, saved.Definition.ID)
		if err != nil {
			return fmt.Errorf("更新 Agent 定义: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO agent_versions (id, workspace_id, agent_id, version_number, configuration, config_checksum, created_by, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		saved.Version.ID, saved.Version.WorkspaceID, saved.Version.AgentID, saved.Version.VersionNumber, configuration, saved.Version.ConfigChecksum, saved.Version.CreatedBy, saved.Version.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("保存 Agent 版本: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_definitions SET current_version_id = $1, updated_at = $2 WHERE id = $3`, saved.Version.ID, saved.Definition.UpdatedAt, saved.Definition.ID); err != nil {
		return fmt.Errorf("更新 Agent 当前版本: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("提交 Agent 配置事务: %w", err)
	}
	return nil
}

func (r *PGRepository) SetStatus(ctx context.Context, actor auth.Principal, agentID string, status Status) error {
	definition, err := r.Find(ctx, actor, agentID)
	if err != nil {
		return err
	}
	if err := checkManageForSave(actor, definition); err != nil {
		return err
	}
	var deletedAt any
	if status == StatusDeleted {
		deletedAt = time.Now().UTC()
	}
	result, err := r.pool.Exec(ctx, `UPDATE agent_definitions SET status=$1, deleted_at=$2, updated_at=now() WHERE id=$3 AND workspace_id=$4 AND deleted_at IS NULL`, status, deletedAt, agentID, actor.WorkspaceID)
	if err != nil {
		return fmt.Errorf("更新 Agent 状态: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func definitionQuery() string {
	return `SELECT d.id, d.workspace_id, d.owner_id, d.owner_type, COALESCE(d.current_version_id, ''), d.name,
		d.avatar, d.description, d.tags, d.visibility, d.status, d.is_default, d.created_by, d.created_at, d.updated_at, d.deleted_at
		FROM agent_definitions d `
}

type definitionScanner interface{ Scan(dest ...any) error }

func (r *PGRepository) scanDefinition(ctx context.Context, row definitionScanner) (AgentDefinition, error) {
	return scanDefinitionRow(row)
}

func scanDefinitionRow(row definitionScanner) (AgentDefinition, error) {
	var definition AgentDefinition
	var tags []byte
	if err := row.Scan(&definition.ID, &definition.WorkspaceID, &definition.OwnerID, &definition.OwnerType, &definition.CurrentVersionID,
		&definition.Name, &definition.Avatar, &definition.Description, &tags, &definition.Visibility, &definition.Status,
		&definition.IsDefault, &definition.CreatedBy, &definition.CreatedAt, &definition.UpdatedAt, &definition.DeletedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AgentDefinition{}, ErrNotFound
		}
		return AgentDefinition{}, fmt.Errorf("读取 Agent: %w", err)
	}
	if err := json.Unmarshal(tags, &definition.Tags); err != nil {
		return AgentDefinition{}, fmt.Errorf("解析 Agent 标签: %w", err)
	}
	return definition, nil
}

func checkManageForSave(actor auth.Principal, definition AgentDefinition) error {
	if definition.WorkspaceID != actor.WorkspaceID || (definition.OwnerType == "user" && definition.OwnerID != actor.UserID) || (definition.OwnerType == "system" && !auth.HasRole(actor, "admin")) {
		return ErrForbidden
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var constraint interface{ SQLState() string }
	return errors.As(err, &constraint) && constraint.SQLState() == "23505"
}

var _ Repository = (*PGRepository)(nil)
