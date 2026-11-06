package memory

import (
	"context"
	"strings"
	"time"

	"linq-bs/server/internal/framework/auth"
)

// DeadJob 只暴露可用于重试的任务元数据，错误原文不进入 API。
type DeadJob struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	LastErrorSummary string    `json:"last_error_summary"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// jobErrorSummary 采用固定类别白名单，未知错误也不截取原文，避免泄露请求或模型内容。
func jobErrorSummary(raw string) string {
	switch {
	case strings.Contains(raw, "context deadline exceeded"):
		return "任务执行超时"
	case strings.Contains(raw, "context canceled"):
		return "任务执行取消"
	case strings.Contains(raw, "请求百炼"), strings.Contains(raw, "百炼模型返回"), strings.Contains(raw, "百炼向量"):
		return "模型服务请求失败"
	case strings.Contains(raw, "JSON"), strings.Contains(raw, "解析模型流"):
		return "模型响应格式无效"
	case strings.Contains(raw, "no rows in result set"):
		return "任务关联数据不存在"
	default:
		return "任务执行失败"
	}
}

func (s *Service) deadJobs(ctx context.Context, p auth.Principal) ([]DeadJob, error) {
	rows, err := s.DB.Query(ctx, `SELECT id,kind,last_error,updated_at FROM memory_jobs WHERE owner_user_id=$1 AND status='dead' ORDER BY updated_at DESC,id DESC LIMIT 20`, p.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []DeadJob{}
	for rows.Next() {
		var job DeadJob
		var raw string
		if err := rows.Scan(&job.ID, &job.Kind, &raw, &job.UpdatedAt); err != nil {
			return nil, err
		}
		job.LastErrorSummary = jobErrorSummary(raw)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
