//go:build integration

package bootstrap_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"linq-bs/server/internal/agent/memory"
	agenthttp "linq-bs/server/internal/agent/transport/http"
	"linq-bs/server/internal/framework/auth"
)

// testMemoryAPIFixes 经真实 Handler、数据库和百炼验证新增、重复事实和 dead 重试。
func testMemoryAPIFixes(t *testing.T, ctx context.Context, mem *memory.Service, p, other auth.Principal) {
	t.Helper()
	h := &agenthttp.MemoryHandler{Memory: mem, Agents: mem.Agents}
	request := func(actor auth.Principal, method, path, id, body string, handler http.HandlerFunc) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(auth.WithPrincipal(ctx, actor))
		r.SetPathValue("job_id", id)
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	add := func(in memory.AddInput, status int) memory.Item {
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatal(err)
		}
		w := request(p, "POST", "/api/v1/memories", "", string(b), h.Add)
		if w.Code != status {
			t.Fatalf("新增 status=%d want=%d", w.Code, status)
		}
		var item memory.Item
		if status == 201 {
			if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
				t.Fatal(err)
			}
			if !regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`).MatchString(item.SubjectKey) {
				t.Fatal("生成键不合法")
			}
		}
		return item
	}
	first := memory.AddInput{Content: "我最喜欢的水果是苹果。", Namespace: "preferences", Importance: 8}
	a := add(first, 201)
	b := add(memory.AddInput{Content: "我周末喜欢骑自行车。", Namespace: "preferences", Importance: 8}, 201)
	if a.SubjectKey == b.SubjectKey {
		t.Fatal("不同事实使用相同键")
	}
	add(first, 409)
	// 显式键保持原有冲突行为；修改原键让自动生成必然与不同事实碰撞。
	explicit := first
	explicit.SubjectKey = a.SubjectKey
	explicit.Content = "我喜欢冬天。"
	add(explicit, 409)
	if _, err := mem.Edit(ctx, p, a.ID, memory.EditInput{Content: "我最喜欢的水果是香蕉。", Namespace: "preferences", Importance: 8}); err != nil {
		t.Fatal(err)
	}
	c := add(first, 201)
	if c.SubjectKey == a.SubjectKey {
		t.Fatal("不同事实键冲突未添加后缀")
	}
	stats := func(actor auth.Principal) memory.Stats {
		w := request(actor, "GET", "/api/v1/memory/stats", "", "", h.Stats)
		if w.Code != 200 {
			t.Fatalf("stats status=%d", w.Code)
		}
		var out memory.Stats
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if out.DeadJobs == nil {
			t.Fatal("dead_jobs 必须为数组")
		}
		return out
	}
	if len(stats(p).DeadJobs) != 0 {
		t.Fatal("初始 dead_jobs 非空")
	}
	// 按验收要求构造 dead 任务，实际 SQL 触发器负责记录更新时间。
	for i := 0; i < 22; i++ {
		id := fmt.Sprintf("api_dead_%s_%02d", p.UserID, i)
		if _, err := mem.DB.Exec(ctx, `INSERT INTO memory_jobs(id,kind,ref,owner_user_id,status,last_error) VALUES($1,'maintain',$1,$2,'dead',$3)`, id, p.UserID, `request_body contains private-memory-marker and credential-marker`); err != nil {
			t.Fatal(err)
		}
	}
	out := stats(p)
	if out.Dead != 22 || len(out.DeadJobs) != 20 {
		t.Fatalf("dead=%d jobs=%d", out.Dead, len(out.DeadJobs))
	}
	for i, job := range out.DeadJobs {
		if job.Kind != "maintain" || job.LastErrorSummary != "任务执行失败" || job.UpdatedAt.IsZero() {
			t.Fatal("dead 元数据或脱敏不正确")
		}
		if i > 0 && out.DeadJobs[i-1].UpdatedAt.Before(job.UpdatedAt) {
			t.Fatal("dead_jobs 排序不正确")
		}
	}
	if len(stats(other).DeadJobs) != 0 {
		t.Fatal("泄露其他用户任务")
	}
	id := out.DeadJobs[0].ID
	if w := request(other, "POST", "/api/v1/memory/jobs/"+id+"/retry", id, "", h.Retry); w.Code != 404 {
		t.Fatal("跨用户重试未拒绝")
	}
	if w := request(p, "POST", "/api/v1/memory/jobs/"+id+"/retry", id, "", h.Retry); w.Code != 204 {
		t.Fatal("重试失败")
	}
	out = stats(p)
	if out.Dead != 21 {
		t.Fatal("重试后 dead 数量错误")
	}
	for _, job := range out.DeadJobs {
		if job.ID == id {
			t.Fatal("重试任务仍在 dead_jobs 中")
		}
	}
	// 清理构造的任务，避免后续 S1 的 worker 领取它们。
	if _, err := mem.DB.Exec(ctx, `DELETE FROM memory_jobs WHERE owner_user_id=$1`, p.UserID); err != nil {
		t.Fatal(err)
	}
}
