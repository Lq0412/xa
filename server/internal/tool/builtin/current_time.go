package builtin

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"linq-bs/server/internal/tool"
)

const maxToolDelayMS = 5000

var schemaCurrentTimeInput = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`)
var schemaCurrentTimeConfig = json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"timezone":{"type":"string","description":"IANA 时区，例如 Asia/Shanghai"},"delay_ms":{"type":"integer","description":"验收/E2E 用人为延迟毫秒","minimum":0,"maximum":5000}}}`)

type CurrentTime struct{}

func (CurrentTime) Spec() tool.Spec {
	return tool.Spec{
		Name: "current_time", Version: "1", Title: "当前时间", Description: descCurrentTime,
		InputSchema: schemaCurrentTimeInput, ConfigSchema: schemaCurrentTimeConfig,
		Effect: tool.EffectRead, Idempotent: true, DefaultEnabled: true,
		Approval:       tool.ApprovalPolicy{Default: tool.ApprovalNever, Overridable: true},
		DefaultTimeout: 12 * time.Second, MaxCallsPerRun: 32, Available: true,
	}
}

func (CurrentTime) Execute(ctx context.Context, call tool.Call, env tool.Env) (tool.Result, error) {
	_ = ctx
	tz := "Asia/Shanghai"
	var delay time.Duration
	if len(env.Config) > 0 {
		var cfg struct {
			Timezone string `json:"timezone"`
			DelayMS  int    `json:"delay_ms"`
		}
		if err := json.Unmarshal(env.Config, &cfg); err == nil {
			if cfg.Timezone != "" {
				tz = cfg.Timezone
			}
			if cfg.DelayMS > maxToolDelayMS {
				cfg.DelayMS = maxToolDelayMS
			}
			if cfg.DelayMS > 0 && os.Getenv("XA_TOOL_DELAY_ALLOWED") == "1" {
				delay = time.Duration(cfg.DelayMS) * time.Millisecond
			}
		}
	}
	if delay > 0 {
		select {
		case <-ctx.Done():
			return tool.Result{}, ctx.Err()
		case <-time.After(delay):
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
