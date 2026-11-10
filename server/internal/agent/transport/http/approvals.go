package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	frameworkhttp "linq-bs/server/internal/framework/http"
)

// ApprovalRecords 只读审批记录，权限在后端独立检查，游标按创建时间与 id 排序。
func (h *RunHandler) ApprovalRecords(w http.ResponseWriter, r *http.Request) {
	p, ok := requirePrincipal(w, r)
	if !ok {
		return
	}
	if !slices.Contains(p.Roles, "admin") {
		frameworkhttp.WriteError(w, r, 403, "forbidden", fmt.Errorf("仅管理员可查看审批记录"))
		return
	}
	status, toolName, cursor := r.URL.Query().Get("status"), r.URL.Query().Get("tool_name"), r.URL.Query().Get("cursor")
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			frameworkhttp.WriteError(w, r, 400, "invalid_limit", fmt.Errorf("limit 无效"))
			return
		}
		limit = n
	}
	if status != "" && !slices.Contains([]string{"pending", "approved", "rejected", "expired", "cancelled"}, status) {
		frameworkhttp.WriteError(w, r, 400, "invalid_status", fmt.Errorf("审批状态无效"))
		return
	}
	rows, err := h.Runs.DB.Query(r.Context(), `SELECT a.id,a.created_at,u.email,r.conversation_id,r.agent_id,d.name,a.tool_name,a.display,a.status,extract(epoch FROM (coalesce(a.decided_at,now())-a.created_at))*1000,coalesce(decider.email,''),a.remember,a.run_id
FROM run_approvals a JOIN runs r ON r.id=a.run_id JOIN users u ON u.id=a.owner_user_id JOIN agent_definitions d ON d.id=r.agent_id LEFT JOIN users decider ON decider.id=a.decided_by
WHERE r.workspace_id=$1 AND ($2='' OR a.status=$2) AND ($3='' OR a.tool_name=$3)
AND ($4='' OR (a.created_at,a.id)<(SELECT created_at,id FROM run_approvals WHERE id=$4))
ORDER BY a.created_at DESC,a.id DESC LIMIT $5`, p.WorkspaceID, status, toolName, cursor, limit+1)
	if err != nil {
		frameworkhttp.WriteError(w, r, 500, "approvals_unavailable", err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, email, conv, agentID, agent, name, status, decider, remember, rid string
		var at time.Time
		var display []byte
		var wait float64
		if err := rows.Scan(&id, &at, &email, &conv, &agentID, &agent, &name, &display, &status, &wait, &decider, &remember, &rid); err != nil {
			frameworkhttp.WriteError(w, r, 500, "approvals_unavailable", err)
			return
		}
		var preview map[string]any
		if err := json.Unmarshal(display, &preview); err != nil {
			frameworkhttp.WriteError(w, r, 500, "approvals_unavailable", err)
			return
		}
		items = append(items, map[string]any{"id": id, "created_at": at, "user": email, "conversation_id": conv, "agent_id": agentID, "agent": agent, "tool_name": name, "title": preview["title"], "summary": preview["summary"], "status": status, "wait_ms": int64(wait), "decider": decider, "remember": remember, "run_id": rid})
	}
	if err := rows.Err(); err != nil {
		frameworkhttp.WriteError(w, r, 500, "approvals_unavailable", err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1]["id"].(string)
	}
	frameworkhttp.WriteJSON(w, 200, map[string]any{"approvals": items, "next_cursor": next})
}
