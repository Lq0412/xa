package conversation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MessagePart 与 web-user MessagePart 对齐（JSON 字段名一致）。
type MessagePart map[string]any

// partsReplayOpts 控制历史组装时是否保留思考全文。
type partsReplayOpts struct {
	stripReasoningText bool
}

// ListRunEventsByRunIDs 批量读取 run 事件（按 run_id、seq 升序）。
func listRunEventsByRunIDs(ctx context.Context, db *pgxpool.Pool, runIDs []string) (map[string][]replayEvent, error) {
	out := map[string][]replayEvent{}
	if len(runIDs) == 0 {
		return out, nil
	}
	rows, err := db.Query(ctx, `
SELECT run_id, seq, type, payload FROM run_events
WHERE run_id = ANY($1)
ORDER BY run_id ASC, seq ASC`, runIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var runID, typ string
		var seq int64
		var payload []byte
		if err := rows.Scan(&runID, &seq, &typ, &payload); err != nil {
			return nil, err
		}
		out[runID] = append(out[runID], replayEvent{Seq: seq, Type: typ, Payload: payload})
	}
	return out, rows.Err()
}

type replayEvent struct {
	Seq     int64
	Type    string
	Payload []byte
}

func envelopeType(payload []byte) (typ string, data map[string]any) {
	_ = json.Unmarshal(payload, &data)
	typ, _ = data["type"].(string)
	if typ == "" {
		return "", data
	}
	return typ, data
}

// buildPartsFromEvents 将 v2 事件重放为 parts（与前端 applyRunEvent 顺序一致）。
func buildPartsFromEvents(events []replayEvent, opts partsReplayOpts) []MessagePart {
	var parts []MessagePart
	pushText := func(delta string) {
		if delta == "" {
			return
		}
		if len(parts) > 0 {
			if last, ok := parts[len(parts)-1]["type"].(string); ok && last == "text" {
				parts[len(parts)-1]["text"] = parts[len(parts)-1]["text"].(string) + delta
				return
			}
		}
		parts = append(parts, MessagePart{"type": "text", "text": delta})
	}
	for _, ev := range events {
		typ, data := envelopeType(ev.Payload)
		if typ == "" {
			typ = ev.Type
		}
		switch typ {
		case "message.delta":
			pushText(fmt.Sprint(data["delta"]))
		case "reasoning.delta":
			if opts.stripReasoningText {
				continue
			}
			itemID := fmt.Sprint(data["item_id"])
			if itemID == "" || itemID == "<nil>" {
				itemID = "reason"
			}
			idx := -1
			for i, p := range parts {
				if p["type"] == "reasoning" && p["item_id"] == itemID {
					idx = i
					break
				}
			}
			if idx < 0 {
				parts = append(parts, MessagePart{
					"type": "reasoning", "item_id": itemID, "text": "", "status": "streaming",
				})
				idx = len(parts) - 1
			}
			cur, _ := parts[idx]["text"].(string)
			parts[idx]["text"] = cur + fmt.Sprint(data["delta"])
		case "reasoning.completed":
			itemID := fmt.Sprint(data["item_id"])
			for i, p := range parts {
				if p["type"] == "reasoning" && (itemID == "" || p["item_id"] == itemID) {
					parts[i]["status"] = "completed"
					if opts.stripReasoningText {
						parts[i]["text"] = ""
					}
					if v, ok := data["truncated"].(bool); ok {
						parts[i]["truncated"] = v
					}
				}
			}
			if opts.stripReasoningText {
				// 历史列表仍展示折叠占位，不含正文。
				found := false
				for _, p := range parts {
					if p["type"] == "reasoning" {
						found = true
						break
					}
				}
				if !found {
					if itemID == "" || itemID == "<nil>" {
						itemID = "reason"
					}
					parts = append(parts, MessagePart{
						"type": "reasoning", "item_id": itemID, "text": "", "status": "completed",
					})
				}
			}
		case "tool.preparing", "tool.called", "tool.started", "tool.completed":
			callID, _ := data["call_id"].(string)
			indexF, _ := data["index"].(float64)
			var part MessagePart
			partIdx := -1
			name := fmt.Sprint(data["name"])
			for i, p := range parts {
				if p["type"] != "tool" {
					continue
				}
				if callID != "" && fmt.Sprint(p["call_id"]) == callID {
					partIdx = i
					break
				}
				if callID == "" && data["index"] != nil {
					if pi, ok := p["index"].(float64); ok && int(indexF) == int(pi) {
						partIdx = i
						break
					}
				}
				if partIdx < 0 && name != "" && fmt.Sprint(p["name"]) == name {
					partIdx = i
				}
			}
			if partIdx >= 0 {
				part = parts[partIdx]
			} else {
				part = MessagePart{
					"type":   "tool",
					"name":   fmt.Sprint(data["name"]),
					"title":  fmt.Sprint(firstNonEmpty(data["title"], data["name"], "工具")),
					"status": "preparing",
				}
				if callID != "" {
					part["call_id"] = callID
				}
				if indexF > 0 || data["index"] != nil {
					part["index"] = data["index"]
				}
				parts = append(parts, part)
				partIdx = len(parts) - 1
			}
			switch typ {
			case "tool.preparing":
				part["status"] = "preparing"
			case "tool.called":
				part["call_id"] = callID
				st := fmt.Sprint(data["status"])
				if st == "rejected" {
					part["status"] = "rejected"
				} else {
					part["status"] = "pending"
				}
				if ap, ok := data["args_preview"].(string); ok && ap != "" {
					part["args_preview"] = ap
				}
			case "tool.started":
				part["status"] = "running"
			case "tool.completed":
				st := fmt.Sprint(data["status"])
				if st == "" {
					st = "succeeded"
				}
				part["status"] = st
				if s, ok := data["summary"].(string); ok {
					part["summary"] = s
				}
				if items, ok := data["items"].([]any); ok {
					part["items"] = items
				}
				if d, ok := data["duration_ms"].(float64); ok {
					part["duration_ms"] = int(d)
				}
			}
			parts[partIdx] = part
		}
	}
	return parts
}

func firstNonEmpty(vals ...any) string {
	for _, v := range vals {
		s := fmt.Sprint(v)
		if s != "" && s != "<nil>" {
			return s
		}
	}
	return ""
}

func attachPartsToMessages(ctx context.Context, db *pgxpool.Pool, messages []Message) error {
	runIDs := make([]string, 0)
	for i := range messages {
		if messages[i].Role != "assistant" || messages[i].RunID == "" {
			continue
		}
		runIDs = append(runIDs, messages[i].RunID)
	}
	if len(runIDs) == 0 {
		return nil
	}
	uniq := map[string]struct{}{}
	ids := make([]string, 0, len(runIDs))
	for _, id := range runIDs {
		if _, ok := uniq[id]; ok {
			continue
		}
		uniq[id] = struct{}{}
		ids = append(ids, id)
	}
	byRun, err := listRunEventsByRunIDs(ctx, db, ids)
	if err != nil {
		return err
	}
	partsByRun := map[string][]MessagePart{}
	for runID, evs := range byRun {
		partsByRun[runID] = buildPartsFromEvents(evs, partsReplayOpts{stripReasoningText: true})
	}
	for i := range messages {
		rid := messages[i].RunID
		if rid == "" {
			continue
		}
		parts := partsByRun[rid]
		if len(parts) == 0 {
			continue
		}
		// 若重放未包含文本段，用 content 兜底，避免只有工具时气泡空白。
		hasText := false
		for _, p := range parts {
			if p["type"] == "text" {
				hasText = true
				break
			}
		}
		if !hasText && messages[i].Content != "" {
			parts = append(parts, MessagePart{"type": "text", "text": messages[i].Content})
		}
		messages[i].Parts = parts
	}
	return nil
}
