package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	frameworkhttp "linq-bs/server/internal/framework/http"
)

type approvalRecordsInput struct {
	Status   string `query:"status" enum:"pending,approved,rejected,expired,cancelled" doc:"按审批状态筛选"`
	ToolName string `query:"tool_name" doc:"按工具名称筛选"`
	Cursor   string `query:"cursor" doc:"上一页返回的 next_cursor"`
	Limit    string `query:"limit" doc:"每页条数，范围 1 至 100，默认 50"`
}

type approvalRecord struct {
	ID             string          `json:"id"`
	CreatedAt      time.Time       `json:"created_at"`
	User           string          `json:"user"`
	ConversationID string          `json:"conversation_id"`
	AgentID        string          `json:"agent_id"`
	Agent          string          `json:"agent"`
	ToolName       string          `json:"tool_name"`
	Title          json.RawMessage `json:"title"`
	Summary        json.RawMessage `json:"summary"`
	Status         string          `json:"status"`
	WaitMS         int64           `json:"wait_ms"`
	Decider        string          `json:"decider"`
	Remember       string          `json:"remember"`
	RunID          string          `json:"run_id"`
}

type approvalRecordsPage struct {
	Approvals  []approvalRecord `json:"approvals" nullable:"false"`
	NextCursor string           `json:"next_cursor"`
}

// ApprovalRecords 只读审批记录，权限在后端独立检查，游标按创建时间与 id 排序。
func (h *RunHandler) ApprovalRecords(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	output, err := h.approvalRecords(r.Context(), &approvalRecordsInput{
		Status: query.Get("status"), ToolName: query.Get("tool_name"), Cursor: query.Get("cursor"), Limit: query.Get("limit"),
	})
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) approvalRecords(ctx context.Context, input *approvalRecordsInput) (*routeOutput[approvalRecordsPage], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(p.Roles, "admin") {
		return nil, frameworkhttp.APIError(ctx, 403, "forbidden", fmt.Errorf("仅管理员可查看审批记录"))
	}
	limit := 50
	if input.Limit != "" {
		n, err := strconv.Atoi(input.Limit)
		if err != nil || n < 1 || n > 100 {
			return nil, frameworkhttp.APIError(ctx, 400, "invalid_limit", fmt.Errorf("limit 无效"))
		}
		limit = n
	}
	if input.Status != "" && !slices.Contains([]string{"pending", "approved", "rejected", "expired", "cancelled"}, input.Status) {
		return nil, frameworkhttp.APIError(ctx, 400, "invalid_status", fmt.Errorf("审批状态无效"))
	}
	rows, err := h.Runs.DB.Query(ctx, `SELECT a.id,a.created_at,u.email,r.conversation_id,r.agent_id,d.name,a.tool_name,a.display,a.status,extract(epoch FROM (coalesce(a.decided_at,now())-a.created_at))*1000,coalesce(decider.email,''),a.remember,a.run_id
FROM run_approvals a JOIN runs r ON r.id=a.run_id JOIN users u ON u.id=a.owner_user_id JOIN agent_definitions d ON d.id=r.agent_id LEFT JOIN users decider ON decider.id=a.decided_by
WHERE r.workspace_id=$1 AND ($2='' OR a.status=$2) AND ($3='' OR a.tool_name=$3)
AND ($4='' OR (a.created_at,a.id)<(SELECT created_at,id FROM run_approvals WHERE id=$4))
ORDER BY a.created_at DESC,a.id DESC LIMIT $5`, p.WorkspaceID, input.Status, input.ToolName, input.Cursor, limit+1)
	if err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
	}
	defer rows.Close()
	items := []approvalRecord{}
	for rows.Next() {
		var item approvalRecord
		var display []byte
		var wait float64
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.User, &item.ConversationID, &item.AgentID, &item.Agent, &item.ToolName, &display,
			&item.Status, &wait, &item.Decider, &item.Remember, &item.RunID); err != nil {
			return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
		}
		var preview map[string]json.RawMessage
		if err := json.Unmarshal(display, &preview); err != nil {
			return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
		}
		item.Title, item.Summary, item.WaitMS = preview["title"], preview["summary"], int64(wait)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	return &routeOutput[approvalRecordsPage]{Body: approvalRecordsPage{Approvals: items, NextCursor: next}}, nil
}
