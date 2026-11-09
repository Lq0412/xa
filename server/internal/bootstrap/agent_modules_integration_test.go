//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/agent/runtime"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/framework/postgres"
	"linq-bs/server/internal/system/user"
	"strings"
	"testing"
	"time"
)

// cleanupModuleUser 只在一个事务中清理本次验收账号，满足主 Agent 的延迟约束。
func cleanupModuleUser(t *testing.T, db *pgxpool.Pool, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Error(err)
		return
	}
	defer tx.Rollback(ctx)
	for _, query := range []string{
		"DELETE FROM conversations WHERE user_id=$1",
		"UPDATE agent_definitions SET current_version_id=NULL WHERE owner_id=$1",
		"DELETE FROM agent_versions WHERE agent_id IN (SELECT id FROM agent_definitions WHERE owner_id=$1)",
		"DELETE FROM agent_definitions WHERE owner_id=$1",
		"DELETE FROM users WHERE id=$1",
	} {
		if _, err := tx.Exec(ctx, query, id); err != nil {
			t.Error(err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Error(err)
	}
}

// TestAgentModules 使用真实 PostgreSQL 和百炼低成本模型验证完整配置链路。
func TestAgentModules(t *testing.T) {
	cfg := loadConfig(t)
	db, err := postgres.Open(t.Context(), cfg.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	users := user.NewService(user.NewPGRepository(db))
	account, err := users.Register(t.Context(), user.Credentials{Email: "modules-" + strings.ToLower(rand.Text()) + "@example.com", Password: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupModuleUser(t, db, account.ID)
	actor := auth.Principal{UserID: account.ID, WorkspaceID: account.WorkspaceID}
	agents := definition.NewService(definition.NewPGRepository(db), cfg.ChatModels)
	library, err := (&definition.Library{DB: db}).List(t.Context(), actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(library) != 1 || !library[0].IsPrimary || library[0].OwnerUserID != account.ID {
		t.Fatalf("注册主 Agent 错误: %+v", library)
	}
	primary := library[0]
	if err := agents.Delete(t.Context(), actor, primary.ID); !errors.Is(err, definition.ErrForbidden) {
		t.Fatalf("主 Agent 可以删除: %v", err)
	}
	if _, err := db.Exec(t.Context(), "UPDATE agent_definitions SET is_primary=false WHERE id=$1", primary.ID); err == nil {
		t.Fatal("数据库允许移除主 Agent 标志")
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO agent_definitions(id,workspace_id,owner_id,owner_user_id,owner_type,name,visibility,is_primary,created_by) VALUES($1,$2,$3,$3,'user','重复主 Agent','private',true,$3)`, "agt_"+rand.Text(), account.WorkspaceID, account.ID); err == nil {
		t.Fatal("数据库允许重复主 Agent")
	}
	// 回滚用户插入必须同时回滚触发器创建的主 Agent。
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rolledID := "usr_" + rand.Text()
	if _, err := tx.Exec(t.Context(), `INSERT INTO users(id,workspace_id,email,password_hash,display_name) VALUES($1,$2,$3,'unused','rollback')`, rolledID, account.WorkspaceID, rolledID+"@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM agent_definitions WHERE owner_user_id=$1", rolledID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("注册事务不完整: %d %v", count, err)
	}
	first, err := agents.GetVersion(t.Context(), actor, primary.ID, primary.CurrentVersionID)
	if err != nil {
		t.Fatal(err)
	}
	input := definition.SaveInput{AgentID: primary.ID, ExpectedVersionID: first.ID, Name: primary.Name, Visibility: primary.Visibility, Configuration: first.Configuration}
	input.Configuration.SoulMD = "你是一位诗人。每次回答都用两句简短的中文诗，第一句以「诗意：」开头。"
	input.Configuration.Model = "qwen3.8-flash"
	poet, err := agents.Save(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	if poet.Version.ID == first.ID || poet.Version.VersionNumber != first.VersionNumber+1 {
		t.Fatal("没有生成新版本")
	}
	old, err := agents.GetVersion(t.Context(), actor, primary.ID, first.ID)
	if err != nil || old.Configuration.SoulMD != first.Configuration.SoulMD {
		t.Fatal("旧版本被覆盖")
	}
	if _, err := agents.Save(t.Context(), actor, input); !errors.Is(err, definition.ErrConflict) {
		t.Fatalf("未拒绝旧版本: %v", err)
	}
	input.ExpectedVersionID = poet.Version.ID
	for _, invalidModel := range []string{"unknown-model", " qwen3.8-flash", " "} {
		input.Configuration.Model = invalidModel
		if _, err := agents.Save(t.Context(), actor, input); !errors.Is(err, definition.ErrInvalid) {
			t.Fatalf("未拒绝未知模型 %q: %v", invalidModel, err)
		}
	}
	chats := newChatService(t, db, agents, nil)
	chat, err := chats.GetOrCreate(t.Context(), actor, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	same, err := chats.GetOrCreate(t.Context(), actor, primary.ID)
	if err != nil || same.ID != chat.ID {
		t.Fatal("创建了重复会话")
	}
	answer, _, err := chats.Send(t.Context(), actor, chat.ID, "你好", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer.Content, "诗意") || answer.Model != "qwen3.8-flash" {
		t.Fatalf("SOUL 或模型未生效: %s", answer.Content)
	}
	input.Configuration.Model = "qwen3.8-flash"
	input.Configuration.SoulMD = "你是极简助手。无论用户说什么，只回答「收到」。不要沿用历史回复风格。"
	concise, err := agents.Save(t.Context(), actor, input)
	if err != nil {
		t.Fatal(err)
	}
	answer, _, err = chats.Send(t.Context(), actor, chat.ID, "你好", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Trim(answer.Content, "。！! \n") != "收到" {
		t.Fatalf("新版 SOUL 未生效: %s", answer.Content)
	}
	page, err := chats.Messages(t.Context(), actor, chat.ID, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || !page.HasMore || page.Before == "" {
		t.Fatalf("分页错误: %+v", page)
	}
	earlier, err := chats.Messages(t.Context(), actor, chat.ID, page.Before, 2)
	if err != nil || len(earlier.Messages) != 2 || earlier.HasMore {
		t.Fatalf("历史分页错误: %+v %v", earlier, err)
	}
	var raw []byte
	var versionID string
	if err := db.QueryRow(t.Context(), "SELECT context_segments,agent_version_id FROM messages WHERE id=$1", answer.ID).Scan(&raw, &versionID); err != nil {
		t.Fatal(err)
	}
	var segments []runtime.Segment
	if err := json.Unmarshal(raw, &segments); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, seg := range segments {
		names[seg.Name] = true
	}
	for _, req := range []string{"platform", "soul", "current"} {
		if !names[req] {
			t.Fatalf("运行上下文缺少 %s: %s", req, raw)
		}
	}
	if versionID != concise.Version.ID {
		t.Fatalf("agent_version_id 错误: %s", versionID)
	}
}
