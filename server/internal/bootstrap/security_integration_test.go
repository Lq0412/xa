//go:build integration

package bootstrap_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/framework/redis"
	"linq-bs/server/internal/system/user"
)

func TestPrivateAgentBelongsToCreatorAndSessionUsesCurrentUser(t *testing.T) {
	cfg := loadConfig(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cache, err := redis.Open(ctx, cfg.Redis)
	if err != nil {
		t.Fatal(err)
	}
	defer cache.Close()

	workspaceID := "integration_" + uuid.NewString()
	adminOne := "usr_" + uuid.NewString()
	adminTwo := "usr_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO workspaces (id, name) VALUES ($1, $1)`, workspaceID); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), `DELETE FROM agent_definitions WHERE workspace_id = $1`, workspaceID)
	defer db.Exec(context.Background(), `DELETE FROM users WHERE workspace_id = $1`, workspaceID)
	defer db.Exec(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)

	agentService := definition.NewService(definition.NewPGRepository(db))
	created, err := agentService.Save(ctx, auth.Principal{UserID: adminOne, WorkspaceID: workspaceID, Roles: []string{"admin"}}, definition.SaveInput{Name: "private", Visibility: definition.VisibilityPrivate})
	if err != nil {
		t.Fatal(err)
	}
	if created.Definition.OwnerType != "user" || created.Definition.OwnerID != adminOne {
		t.Fatalf("私有 Agent 归属错误: %+v", created.Definition)
	}
	list, err := agentService.List(ctx, auth.Principal{UserID: adminTwo, WorkspaceID: workspaceID, Roles: []string{"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("其他管理员看到了私有 Agent: %+v", list)
	}

	if _, err := db.Exec(ctx, `INSERT INTO users (id, workspace_id, email, password_hash, display_name, role, status) VALUES ($1,$2,$3,'unused','one','admin','active')`, adminOne, workspaceID, adminOne+"@example.com"); err != nil {
		t.Fatal(err)
	}
	users := user.NewPGRepository(db)
	sessions := auth.NewRedisSessionStore(cache, time.Hour)
	resolver := auth.NewRedisResolver(sessions, func(ctx context.Context, userID string) (auth.Principal, error) {
		current, err := users.Find(ctx, userID)
		if err != nil {
			return auth.Principal{}, err
		}
		if current.Status != "active" {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return auth.Principal{UserID: current.ID, WorkspaceID: current.WorkspaceID, Roles: []string{current.Role}}, nil
	})
	token, err := sessions.Create(ctx, auth.Principal{UserID: adminOne, WorkspaceID: workspaceID, Roles: []string{"admin"}})
	if err != nil {
		t.Fatal(err)
	}
	defer sessions.Delete(context.Background(), token)
	if _, err := resolver.Resolve(auth.WithSessionToken(ctx, token)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE users SET status = 'disabled' WHERE id = $1`, adminOne); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Resolve(auth.WithSessionToken(ctx, token)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("禁用用户的会话未立即失效: %v", err)
	}
}
