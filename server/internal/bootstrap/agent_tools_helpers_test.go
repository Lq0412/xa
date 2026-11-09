//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/auth"
)

// integrationVector 为真实 PG 集成测试生成确定性 1024 维向量，避免依赖百炼 Embed。
func integrationVector(seed string) []float32 {
	v := make([]float32, 1024)
	sum := sha256.Sum256([]byte(seed))
	for i := range v {
		v[i] = float32(sum[i%32]) / 255
	}
	return v
}

func seedUserEditedMemory(t *testing.T, ctx context.Context, db *pgxpool.Pool, p auth.Principal, namespace, subjectKey, content string) string {
	t.Helper()
	id := "mem_" + uuid.NewString()
	raw, err := json.Marshal(integrationVector(content))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO memory_items(id,workspace_id,owner_user_id,scope,namespace,subject_key,content,kind,importance,pinned,status,source,embedding,sensitive)
VALUES ($1,$2,$3,'user',$4,$5,$6,'fact',8,false,'active','user_edited',$7::vector,false)`,
		id, p.WorkspaceID, p.UserID, namespace, subjectKey, content, string(raw)); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedUserMessage(t *testing.T, ctx context.Context, db *pgxpool.Pool, p auth.Principal, agentID string) string {
	t.Helper()
	convID := "conv_" + uuid.NewString()
	msgID := "msg_" + uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO conversations(id,workspace_id,user_id,agent_id) VALUES ($1,$2,$3,$4)`,
		convID, p.WorkspaceID, p.UserID, agentID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status) VALUES ($1,$2,'user','工具测试','completed')`,
		msgID, convID); err != nil {
		t.Fatal(err)
	}
	return msgID
}
