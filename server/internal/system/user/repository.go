package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"linq-bs/server/internal/framework/workspace"
)

// PGRepository 将用户查询限制在已知工作空间，避免业务层遗漏租户条件。
type PGRepository struct{ pool *pgxpool.Pool }

func NewPGRepository(pool *pgxpool.Pool) *PGRepository { return &PGRepository{pool: pool} }

func (r *PGRepository) Find(ctx context.Context, userID string) (User, error) {
	return r.scan(ctx, r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, email, display_name, role, status
		FROM users WHERE id = $1`, userID))
}

func (r *PGRepository) FindByEmail(ctx context.Context, workspaceID, email string) (User, string, error) {
	var user User
	var passwordHash string
	err := r.pool.QueryRow(ctx, `
		SELECT id, workspace_id, email, display_name, role, status, password_hash
		FROM users WHERE workspace_id = $1 AND lower(email) = lower($2)`, workspaceID, email).
		Scan(&user.ID, &user.WorkspaceID, &user.Email, &user.DisplayName, &user.Role, &user.Status, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("查询用户: %w", err)
	}
	return user, passwordHash, nil
}

func (r *PGRepository) Save(ctx context.Context, user User, passwordHash string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, workspace_id, email, password_hash, display_name, role, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		user.ID, user.WorkspaceID, user.Email, passwordHash, user.DisplayName, user.Role, user.Status)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("保存用户: %w", err)
	}
	return nil
}

// EnsureAdmin 初始化部署管理员；已有同邮箱管理员只更新密码摘要，不创建重复账号。
func (r *PGRepository) EnsureAdmin(ctx context.Context, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("生成管理员密码摘要: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO users (id, workspace_id, email, password_hash, display_name, role, status)
		VALUES ($1, $2, lower($3), $4, 'Administrator', 'admin', 'active')
		ON CONFLICT (workspace_id, email) DO UPDATE SET password_hash = EXCLUDED.password_hash, role = 'admin', status = 'active', updated_at = now()`,
		"usr_"+uuid.NewString(), workspace.DefaultWorkspaceID, email, string(hash))
	if err != nil {
		return fmt.Errorf("初始化管理员: %w", err)
	}
	return nil
}

type rowScanner interface{ Scan(dest ...any) error }

func (r *PGRepository) scan(ctx context.Context, row rowScanner) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.WorkspaceID, &user.Email, &user.DisplayName, &user.Role, &user.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("查询用户: %w", err)
	}
	return user, nil
}

func isUniqueViolation(err error) bool {
	var constraint interface{ SQLState() string }
	return errors.As(err, &constraint) && constraint.SQLState() == "23505"
}
