//go:build integration

package bootstrap_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/tool"
	"linq-bs/server/internal/tool/builtin"
)

func insertRunAndToolResult(t *testing.T, ctx context.Context, db *pgxpool.Pool, p auth.Principal, agentID, verID, convID, userMsgID, ref, original string) string {
	t.Helper()
	runID := "run_" + uuid.NewString()
	_, err := db.Exec(ctx, `INSERT INTO conversations(id,workspace_id,user_id,agent_id) VALUES ($1,$2,$3,$4)
ON CONFLICT (id) DO NOTHING`, convID, p.WorkspaceID, p.UserID, agentID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO messages(id,conversation_id,role,content,status) VALUES ($1,$2,'user','fixture','completed')
ON CONFLICT (id) DO NOTHING`, userMsgID, convID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO runs(id,workspace_id,owner_user_id,conversation_id,agent_id,agent_version_id,root_run_id,status,user_message_id,provider,model,stream_protocol,limits,toolset,toolset_hash,started_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'completed',$8,'fake','test',1,'{}','[]','',now())`,
		runID, p.WorkspaceID, p.UserID, convID, agentID, verID, runID, userMsgID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO tool_results(id,run_id,conversation_id,owner_user_id,step,call_index,call_id,tool_name,effect,status,idempotency_key,model_content,total_chars)
VALUES ($1,$2,$3,$4,1,1,'c1','fake_huge','read','succeeded',$1,$5,$6)`,
		ref, runID, convID, p.UserID, original, len([]rune(original)))
	if err != nil {
		t.Fatal(err)
	}
	return runID
}

func TestHugeResultToolResultReadPaginationAndNotFound(t *testing.T) {
	ctx := t.Context()
	db, agents, _, _, owner, other := testToolsEnv(t)
	list, err := agents.List(ctx, owner)
	if err != nil || len(list) == 0 {
		t.Fatal(err)
	}
	agent := list[0]
	reader := builtin.NewToolResultRead(db)
	convID := "conv_tr_" + strings.ToLower(rand.Text())
	userMsgID := "msg_" + uuid.NewString()
	ref := "tr_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	original := strings.Repeat("片段", 6000)
	insertRunAndToolResult(t, ctx, db, owner, agent.ID, agent.CurrentVersionID, convID, userMsgID, ref, original)

	env := tool.Env{Principal: owner, ConversationID: convID, RunID: "run_fixture"}
	var rebuilt strings.Builder
	offset := 0
	for {
		args, _ := json.Marshal(map[string]any{"result_ref": ref, "offset": offset, "limit": 1200})
		res, err := reader.Execute(ctx, tool.Call{Arguments: args}, env)
		if err != nil {
			t.Fatal(err)
		}
		text, _ := res.Data.(map[string]any)["text"].(string)
		rebuilt.WriteString(text)
		hasMore, _ := res.Data.(map[string]any)["has_more"].(bool)
		if !hasMore {
			break
		}
		offset += len([]rune(text))
	}
	if rebuilt.String() != original {
		t.Fatalf("分页拼回长度 %d，期望 %d", len([]rune(rebuilt.String())), len([]rune(original)))
	}

	wrongConv := tool.Env{Principal: owner, ConversationID: "conv_other", RunID: "run_fixture"}
	args, _ := json.Marshal(map[string]any{"result_ref": ref})
	_, err = reader.Execute(ctx, tool.Call{Arguments: args}, wrongConv)
	var te *tool.Error
	if !errors.As(err, &te) || te.Code != "not_found" {
		t.Fatalf("异会话应 not_found: %v", err)
	}
	wrongOwner := tool.Env{Principal: other, ConversationID: convID, RunID: "run_fixture"}
	_, err = reader.Execute(ctx, tool.Call{Arguments: args}, wrongOwner)
	if !errors.As(err, &te) || te.Code != "not_found" {
		t.Fatalf("异主应 not_found: %v", err)
	}
}
