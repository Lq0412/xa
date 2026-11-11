package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	frameworkhttp "linq-bs/server/internal/framework/http"
)

// ActivityCursor 是活动条目的排序位置，也是分页游标的内容。
type ActivityCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type activityItem struct {
	ActivityCursor
	Title  string `json:"title"`
	Result string `json:"result"`
}

// PanelPageInput 是审批和活动列表共用的分页参数。
type PanelPageInput struct {
	Limit  string `query:"limit" doc:"每页条数，范围 1 至 100，默认 20"`
	Cursor string `query:"cursor" doc:"上一页返回的 next_cursor"`
}

type userApprovalsInput struct {
	PanelPageInput
	AgentID string `query:"agent_id" doc:"按 Agent ID 筛选"`
}

type agentActivityInput struct {
	PanelPageInput
	AgentID string `path:"agent_id" doc:"Agent ID"`
}

type userApproval struct {
	ID             string  `json:"id"`
	CreatedAt      string  `json:"created_at" format:"date-time"`
	DecidedAt      *string `json:"decided_at" format:"date-time"`
	ConversationID string  `json:"conversation_id"`
	AgentID        string  `json:"agent_id"`
	Agent          string  `json:"agent"`
	RunID          string  `json:"run_id"`
	ToolName       string  `json:"tool_name"`
	Title          *string `json:"title"`
	Summary        *string `json:"summary"`
	Status         string  `json:"status"`
	Remember       string  `json:"remember"`
	WaitMS         int64   `json:"wait_ms"`
}

type userApprovalPage struct {
	Approvals  []userApproval `json:"approvals" nullable:"false"`
	NextCursor string         `json:"next_cursor"`
}

type activityPage struct {
	Items      []activityItem `json:"items" nullable:"false"`
	NextCursor string         `json:"next_cursor"`
}

// panelPage 校验两个列表共用的时间与 ID 游标。
func panelPage(ctx context.Context, input PanelPageInput) (int, ActivityCursor, huma.StatusError) {
	limit := 20
	if input.Limit != "" {
		n, err := strconv.Atoi(input.Limit)
		if err != nil || n < 1 || n > 100 {
			return 0, ActivityCursor{}, frameworkhttp.APIError(ctx, 400, "invalid_limit", fmt.Errorf("limit 无效"))
		}
		limit = n
	}
	var cursor ActivityCursor
	if input.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(input.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil || cursor.ID == "" || cursor.CreatedAt.IsZero() {
			return 0, ActivityCursor{}, frameworkhttp.APIError(ctx, 400, "invalid_cursor", fmt.Errorf("cursor 无效"))
		}
	}
	return limit, cursor, nil
}

func panelNext(ctx context.Context, cursor ActivityCursor) (string, huma.StatusError) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", frameworkhttp.APIError(ctx, 500, "internal_error", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// UserApprovals 只投影当前用户的审批摘要，工具参数和决定者身份不出站。
func (h *RunHandler) UserApprovals(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	output, err := h.userApprovals(r.Context(), &userApprovalsInput{
		PanelPageInput: PanelPageInput{Limit: query.Get("limit"), Cursor: query.Get("cursor")}, AgentID: query.Get("agent_id"),
	})
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) userApprovals(ctx context.Context, input *userApprovalsInput) (*routeOutput[userApprovalPage], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	limit, cursor, pageErr := panelPage(ctx, input.PanelPageInput)
	if pageErr != nil {
		return nil, pageErr
	}
	rows, err := h.Runs.DB.Query(ctx, `
SELECT a.id,a.created_at,jsonb_build_object(
 'id',a.id,'created_at',a.created_at,'decided_at',a.decided_at,
 'conversation_id',r.conversation_id,'agent_id',r.agent_id,'agent',d.name,'run_id',r.id,
 'tool_name',a.tool_name,'title',a.display->>'title','summary',a.display->>'summary',
 'status',a.status,'remember',a.remember,'wait_ms',(extract(epoch FROM (coalesce(a.decided_at,now())-a.created_at))*1000)::bigint)
FROM run_approvals a JOIN runs r ON r.id=a.run_id JOIN agent_definitions d ON d.id=r.agent_id
WHERE r.workspace_id=$1 AND r.owner_user_id=$2 AND a.owner_user_id=$2 AND ($3='' OR r.agent_id=$3)
 AND ($4='' OR (a.created_at,a.id)<($5,$4))
ORDER BY a.created_at DESC,a.id DESC LIMIT $6`, p.WorkspaceID, p.UserID, input.AgentID, cursor.ID, cursor.CreatedAt, limit+1)
	if err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
	}
	defer rows.Close()
	items := []userApproval{}
	var last ActivityCursor
	next := ""
	for rows.Next() {
		if len(items) == limit {
			next, pageErr = panelNext(ctx, last)
			if pageErr != nil {
				return nil, pageErr
			}
			break
		}
		var raw []byte
		if err := rows.Scan(&last.ID, &last.CreatedAt, &raw); err != nil {
			return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
		}
		// PostgreSQL 已投影允许返回的字段。保留数据库输出的时间字符串。
		var item userApproval
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "approvals_unavailable", err)
	}
	return &routeOutput[userApprovalPage]{Body: userApprovalPage{Approvals: items, NextCursor: next}}, nil
}

// AgentActivity 只允许 Agent 所有者读取。确认和工具结果按同一次调用关联。
func (h *RunHandler) AgentActivity(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	output, err := h.agentActivity(r.Context(), &agentActivityInput{
		PanelPageInput: PanelPageInput{Limit: query.Get("limit"), Cursor: query.Get("cursor")}, AgentID: r.PathValue("agent_id"),
	})
	writeLegacyJSON(w, http.StatusOK, output, err)
}

func (h *RunHandler) agentActivity(ctx context.Context, input *agentActivityInput) (*routeOutput[activityPage], error) {
	p, err := routePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	limit, cursor, pageErr := panelPage(ctx, input.PanelPageInput)
	if pageErr != nil {
		return nil, pageErr
	}
	agentID := input.AgentID
	var owned bool
	err = h.Runs.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_definitions WHERE id=$1 AND workspace_id=$2 AND owner_type='user' AND owner_id=$3)`, agentID, p.WorkspaceID, p.UserID).Scan(&owned)
	if err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "activity_unavailable", err)
	}
	if !owned {
		return nil, frameworkhttp.APIError(ctx, 404, "not_found", fmt.Errorf("未找到 Agent"))
	}
	rows, err := h.Runs.DB.Query(ctx, agentActivitySQL, p.WorkspaceID, p.UserID, agentID, cursor.ID, cursor.CreatedAt, limit+1)
	if err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "activity_unavailable", err)
	}
	defer rows.Close()
	items := []activityItem{}
	next := ""
	for rows.Next() {
		if len(items) == limit {
			next, pageErr = panelNext(ctx, items[len(items)-1].ActivityCursor)
			if pageErr != nil {
				return nil, pageErr
			}
			break
		}
		var item activityItem
		if err := rows.Scan(&item.ID, &item.CreatedAt, &item.Title, &item.Result); err != nil {
			return nil, frameworkhttp.APIError(ctx, 500, "activity_unavailable", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, frameworkhttp.APIError(ctx, 500, "activity_unavailable", err)
	}
	return &routeOutput[activityPage]{Body: activityPage{Items: items, NextCursor: next}}, nil
}

// 记忆事件保留实际内容。成功删除用工具记录识别本次确认，避免同一个动作出现两次。
const agentActivitySQL = `
WITH agent_runs AS (
 SELECT id,user_message_id FROM runs WHERE workspace_id=$1 AND owner_user_id=$2 AND agent_id=$3
), calls AS (
 SELECT t.*,r.user_message_id,a.status AS approval_status
 FROM tool_results t JOIN agent_runs r ON r.id=t.run_id
 LEFT JOIN run_approvals a ON a.run_id=t.run_id AND a.call_id=t.call_id AND a.owner_user_id=$2
 WHERE t.owner_user_id=$2 AND t.status='succeeded' AND t.tool_name IN ('current_time','memory_forget')
), activity(id,created_at,title,result) AS (
 SELECT 'memory:'||e.id,e.created_at,
  CASE e.op WHEN 'DELETE' THEN '忘掉了一条记忆' WHEN 'UPDATE' THEN '更新了记忆' ELSE '记住了' END,
  left(concat_ws(' · ',CASE WHEN e.actor_type='user' AND e.op='DELETE' THEN '自己删除' END,
    CASE WHEN e.op='DELETE' THEN e.before->>'content' ELSE e.after->>'content' END),240)
 FROM memory_events e
 JOIN memory_items m ON m.id=e.memory_id AND m.workspace_id=$1 AND m.owner_user_id=$2
 JOIN agent_definitions d ON d.id=$3
 LEFT JOIN messages msg ON msg.id=e.source_ref
 LEFT JOIN conversations c ON c.id=msg.conversation_id AND c.user_id=$2 AND c.workspace_id=$1
 WHERE e.workspace_id=$1 AND e.owner_user_id=$2 AND e.op IN ('ADD','UPDATE','DELETE')
  AND (CASE WHEN e.actor_type='user' OR c.id IS NULL THEN m.agent_id=$3 OR (m.scope='user' AND d.is_primary)
       ELSE c.agent_id=$3 END)
  AND NOT (e.op='DELETE' AND e.actor_type='agent' AND EXISTS(
   SELECT 1 FROM calls t WHERE t.tool_name='memory_forget' AND t.user_message_id=e.source_ref AND t.arguments->>'memory_id'=e.memory_id))
 UNION ALL
 SELECT 'tool:'||t.id,coalesce(t.ended_at,t.created_at),
  CASE t.tool_name WHEN 'current_time' THEN '查了一次时间' ELSE '忘掉了一条记忆' END,
  CASE t.tool_name WHEN 'current_time' THEN concat_ws(' · ',CASE WHEN t.approval_status='approved' THEN '你已确认' END,t.display->>'summary')
   ELSE concat_ws(' · ',CASE WHEN t.approval_status='approved' THEN '你已确认' END,
        (SELECT part->>'value' FROM jsonb_array_elements(t.display->'items') part WHERE part->>'label'='内容' LIMIT 1)) END
 FROM calls t WHERE t.tool_name IN ('current_time','memory_forget')
 UNION ALL
 SELECT 'approval:'||a.id,a.decided_at,coalesce(nullif(a.display->>'title',''),'工具确认'),
  CASE a.status WHEN 'approved' THEN '你已确认' WHEN 'rejected' THEN '你已拒绝'
    WHEN 'expired' THEN '超时未执行' WHEN 'cancelled' THEN '已取消' END
 FROM run_approvals a JOIN agent_runs r ON r.id=a.run_id
 WHERE a.owner_user_id=$2 AND a.status<>'pending' AND a.decided_at IS NOT NULL
  AND NOT EXISTS(SELECT 1 FROM calls t WHERE t.run_id=a.run_id AND t.call_id=a.call_id)
)
SELECT id,created_at,title,result FROM activity
WHERE ($4='' OR (created_at,id)<($5,$4))
ORDER BY created_at DESC,id DESC LIMIT $6`
