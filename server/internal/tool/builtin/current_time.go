package builtin

import (
	"context"
	"encoding/json"
	"time"

	"linq-bs/server/internal/tool"
)

var schemaCurrentTimeInput = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
var schemaCurrentTimeConfig = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"timezone":{"type":"string","description":"IANA 时区，例如 Asia/Shanghai"}}}`)

type CurrentTime struct{}

func (CurrentTime) Spec() tool.Spec {
	return tool.Spec{
		Name: "current_time", Version: "1", Title: "当前时间", Description: descCurrentTime,
		InputSchema: schemaCurrentTimeInput, ConfigSchema: schemaCurrentTimeConfig,
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 3 * time.Second, MaxCallsPerRun: 32, Available: true,
	}
}

func (CurrentTime) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	_ = ctx
	tz := "Asia/Shanghai"
	if len(env.Config) > 0 {
		var cfg struct {
			Timezone string `json:"timezone"`
		}
		if err := json.Unmarshal(env.Config, &cfg); err == nil && cfg.Timezone != "" {
			tz = cfg.Timezone
		}
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return tool.Result{}, &tool.Error{Code: "invalid_config", Message: "时区无效", Retryable: true}
	}
	now := time.Now().In(loc)
	data := map[string]any{
		"now_rfc3339": now.Format(time.RFC3339),
		"timezone":    tz,
		"weekday":     now.Weekday().String(),
		"unix":        now.Unix(),
	}
	return tool.Result{
		Data:    data,
		Display: tool.Display{Summary: now.Format("2006-01-02 15:04")},
	}, nil
}
