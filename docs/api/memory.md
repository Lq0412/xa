# 记忆系统 v2 API

所有接口均使用 `Authorization: Bearer <token>`。所有者从登录身份获取；不接受请求指定 owner_user_id。管理员不能读取或修改其他用户记忆。以下 JSON 为字段结构示例，ID 和时间由服务端生成。

## 新增接口

| 方法 | 路径 | 成功响应 |
| --- | --- | --- |
| POST | /api/v1/memories | 201，完整 Item |
| POST | /api/v1/memories/{memory_id}/restore | 200，新的完整 Item |
| GET | /api/v1/conversations/{conversation_id}/summary | 200，最新摘要或 summary:null |
| GET | /api/v1/memory/stats | 200，当前用户统计 |
| POST | /api/v1/memory/jobs/{job_id}/retry | 204，无响应体 |

### POST /api/v1/memories

请求：

```json
{
  "content": "用户希望先看结论。",
  "namespace": "preferences",
  "subject_key": "answer.order",
  "kind": "procedure",
  "importance": 8,
  "pinned": false,
  "scope": "user",
  "agent_id": ""
}
```

content、namespace、importance 必填。content 非空且不超过 2000 个 Unicode 字符；importance 为 1–10。namespace 和 subject_key 匹配 `^[a-z][a-z0-9_.-]{0,63}$`。subject_key 默认等于 namespace；kind 默认 fact，可选 fact/preference/episode/procedure；scope 默认 user，可选 user/agent，agent 时必须提供可访问的 agent_id。来源固定为手动编辑。相同作用域、namespace、subject_key 已有 active 条目时返回 409。凭证类内容拒绝保存。

201 响应（完整 Item，其他返回 Item 的接口使用同样结构）：

```json
{
  "id": "mem_generated",
  "workspace_id": "workspace_generated",
  "owner_user_id": "user_generated",
  "scope": "user",
  "agent_id": null,
  "conversation_id": null,
  "namespace": "preferences",
  "subject_key": "answer.order",
  "content": "用户希望先看结论。",
  "kind": "procedure",
  "importance": 8,
  "pinned": false,
  "status": "active",
  "source": "user_edited",
  "source_type": "user_edit",
  "source_ref": null,
  "supersedes_id": null,
  "created_at": "2026-11-05T00:00:00Z",
  "updated_at": "2026-11-05T00:00:00Z",
  "last_used_at": null,
  "retention": 1,
  "strength_days": 60,
  "sensitive": false
}
```

source 保留旧值 auto/user_edited；自动抽取的 source_type 为 chat_extraction。tool/import/delegation 仅为后续来源约定，本次没有对应外部写入 API。状态可为 active/superseded/deleted/archived。

### POST /api/v1/memories/{memory_id}/restore

无请求体。只接受当前用户的 archived/deleted 条目；其他状态返回 409，非本人或不存在返回 404。200 返回上述完整 Item：id 为新 ID，status=active，source=user_edited，source_type=user_edit，supersedes_id 指向被恢复的旧 ID，source_ref=null，retention=1，strength_days=60。原内容、namespace、subject_key、kind、scope、pinned、sensitive 保留，旧版本与 ARCHIVE/DELETE 历史不删除；新增 RESTORE 事件。相同事实已有 active 版本时返回 409。

### GET /api/v1/conversations/{conversation_id}/summary

无请求体。不存在或非本人会话返回 404。200 有摘要时直接返回：

```json
{
  "id": "sum_generated",
  "conversation_id": "conversation_generated",
  "content": "用户正在完成毕业设计，已确认验收日期。",
  "covers_from_message_id": "message_first",
  "covers_until_message_id": "message_last",
  "message_count": 40,
  "estimated_tokens": 27,
  "version": 2,
  "created_at": "2026-11-05T00:00:00Z"
}
```

没有摘要时：

```json
{"summary": null}
```

摘要按版本只追加，覆盖范围累计，原始消息不删除。

### GET /api/v1/memory/stats

无请求体。200 响应：

```json
{
  "status": {"active": 8, "superseded": 2, "deleted": 1, "archived": 1},
  "namespace": {"profile": 5, "preferences": 3},
  "pending": 2,
  "failed": 1,
  "dead": 0,
  "latest_summary_at": "2026-11-05T00:00:00Z",
  "latest_maintain_at": "2026-11-05T00:01:00Z"
}
```

status 统计全部状态，namespace 仅统计 active；不存在的分组省略。pending 包括待重试任务，failed 包括有失败次数的 pending，因此两者可重叠；dead 单独统计。没有成功摘要/维护时，相应 latest 字段省略。统计为空时 status/namespace 为 {}。

### POST /api/v1/memory/jobs/{job_id}/retry

无请求体；只允许任务所有者将 dead 改为 pending。清空 attempts/last_error/completed_at，next_run_at 设为当前时间。成功返回 204，无 JSON；任务不存在、非本人或状态不是 dead 时返回 404。后台再次失败仍执行相同退避规则。

## 扩展的原有接口

### GET /api/v1/memories

无 JSON 请求体。可用查询参数：

| 参数 | 规则 |
| --- | --- |
| scope | 空值、user、agent、session、all_visible |
| agent_id | 普通查询精确过滤 Agent；all_visible 必填，合并 user 层和该 Agent 层 |
| namespace | 精确过滤 |
| status | 默认 active；可选 active/superseded/deleted/archived |
| limit | 分页时默认 50，最大 200；超过 200 按 200，非整数或小于 1 返回 422 |
| cursor | 上页 next_cursor；非当前用户条目 ID 返回 422 |

不传 limit/cursor 且 scope 不是 all_visible 时，保留旧接口完整列表及 pinned、importance、created_at 的排序。显式传分页参数（如 `?cursor=&limit=50`）或使用 all_visible 时按 ID 升序分页。游标是不可变的最后一条 ID，不是快照；下一页需保留原过滤参数。

200 响应：

```json
{
  "items": [],
  "next_cursor": ""
}
```

items 元素为上述完整 Item。还有下一页时 next_cursor 为当前页最后一条 ID；否则为空字符串。旧客户端继续读取 items。

### PUT /api/v1/agents/{agent_id}/memory-policy

旧请求只含 expected_version_id、enabled 及可选 core_budget_tokens/recall_top_k 仍有效。完整请求：

```json
{
  "expected_version_id": "version_current",
  "enabled": true,
  "core_budget_tokens": 800,
  "recall_top_k": 5,
  "min_relevance": 0.35,
  "history_budget_tokens": 6000,
  "summary_trigger_tokens": 3000,
  "summary_budget_tokens": 800,
  "decay_enabled": true
}
```

expected_version_id、enabled 必填，其余省略时保留当前值；旧配置缺失新增字段时采用上述默认值。core_budget_tokens 为 1–8000，recall_top_k 为 1–50，min_relevance 为 0–1，history_budget_tokens 为 100–50000，summary_trigger_tokens 至少 100，summary_budget_tokens 为 100–10000。当前解析器将整数预算和 recall_top_k 的 0 视为默认值。保存创建 Agent 新版本，期望版本不匹配返回 409。

200 响应：

```json
{
  "current_version_id": "version_new",
  "version_number": 3,
  "memory_policy": {
    "enabled": true,
    "core_budget_tokens": 800,
    "recall_top_k": 5,
    "min_relevance": 0.35,
    "history_budget_tokens": 6000,
    "summary_trigger_tokens": 3000,
    "summary_budget_tokens": 800,
    "decay_enabled": true
  }
}
```

## 其余原有接口

| 方法与路径 | 请求 | 成功响应 |
| --- | --- | --- |
| GET /api/v1/agents/{agent_id}/memory | 无请求体 | 200，见下方 core JSON |
| PUT /api/v1/memories/{memory_id} | 见下方编辑 JSON | 200，完整 Item，新 ID 的 supersedes_id 指向旧 ID |
| DELETE /api/v1/memories/{memory_id} | 无请求体 | 204，无响应体；保留审计，重复删除成功 |
| GET /api/v1/memories/{memory_id}/events | 无请求体 | 200，见下方 events JSON |

core 响应：

```json
{"markdown": "# memory.md\n用户事实", "estimated_tokens": 8, "item_ids": ["mem_generated"]}
```

编辑请求（完整编辑字段）：

```json
{"content": "用户希望先看示例。", "namespace": "preferences", "importance": 9, "pinned": true}
```

events 响应：

```json
{
  "events": [{
    "id": 1,
    "memory_id": "mem_generated",
    "source_ref": null,
    "op": "ADD",
    "actor_type": "user",
    "actor_id": "user_generated",
    "before": null,
    "after": {},
    "reason": "",
    "created_at": "2026-11-05T00:00:00Z"
  }]
}
```

before/after 实际为该事件操作前后完整 Item、合并时的 Item 数组或 null；上例 after 的 {} 表示该位置的对象类型，字段与完整 Item 相同。events 沿取代链与合并父版本返回全部相关历史；新增 ARCHIVE/RESTORE/MERGE 和任务失败事件不改变旧字段。

## 错误和兼容性

通用错误 JSON：

```json
{"code": "invalid_agent", "message": "请求参数无效", "request_id": "request_generated"}
```

HTTP 401 表示未登录；422 表示记忆参数校验失败（沿用 invalid_agent）；400 表示 JSON 解码等请求错误；403 操作无权限；404 不存在或不可见；409 版本/活跃事实冲突；服务异常按现有错误处理返回 5xx。code/message 以实际错误类型为准，request_id 用于定位请求。

原有六个接口的方法、路径、旧请求字段、响应字段和所有者隔离保留。新增响应字段可忽略；自动记忆 source 仍输出 auto，手动为 user_edited。列表不带分页参数继续返回完整列表，避免旧客户端静默漏项；新增分页为显式启用。新能力包括 archived、来源类型、衰减元数据和策略字段。手动删除、编辑及关闭记忆的旧调用方式仍可用。
