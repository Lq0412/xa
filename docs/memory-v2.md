# xa 记忆系统 v2：完整版规格（S1 收尾，一次做完）

目标：记忆不再是半成品。聊多少轮都稳定：提示词长度有上限，早期内容有摘要兜底，长期记忆会去重、更新、遗忘、可追溯；
接口和结构为后续工具调用、多 Agent 共享、评测、runs 追踪预留好接入点。效果可以先不精调，但链路必须完整闭环。
基线：main 21f9ee1（006_memory.sql、memory/{service,context,tasks}.go、runtime/context_builder.go）。
现状问题：conversation.Send 固定取最近 20 条消息（无 token 预算，更早的直接丢）；抽取只看单条用户消息；去重比对最近 200 条；
召回全量精确排序；失败任务不重试；没有遗忘；列表无分页。

## 1. 短期记忆：对话窗口 + 滚动摘要（解决“轮次变多”）
- 历史按 history_budget_tokens（默认 6000）限制：超预算从最旧轮开始丢弃，最后一轮仍超长时按消息分配剩余预算并截断，末尾标注“…（已截断）”，标注计入预算。不再强制保留四轮，也不因历史超预算拒绝请求。
- 当前用户消息完整保留，包括首尾空白，不参与历史预算。只有当前消息自身估算超过当前百炼文本模型的 1,000,000 token 上下文上限时，在 SSE 开始前返回 HTTP 422、`message_too_long`，提示“消息太长，超出模型可处理的长度，请缩短或分段发送”。估算口径仍为中文约一字一个 token、ASCII 约四字符一个 token，不替代供应商 tokenizer。
- 新表 conversation_summaries(id, conversation_id, agent_id, owner_user_id, content, covers_from_message_id, covers_until_message_id, message_count, estimated_tokens, version, created_at)，同一会话只追加，最新一条生效。
- 触发：窗口外“未被摘要覆盖”的消息累计超过 summary_trigger_tokens（默认 3000）时，入队 summary 任务（复用持久任务队列，见 §5）。
- 增量摘要：旧摘要 + 新一段被挤出窗口的消息 → 新摘要（qwen3.8-flash，关思考，≤ summary_budget_tokens 默认 800）。内容要求：用户目标、已做决定、未完成事项、关键事实与数字、对话进展；不编造。
- Context Builder 新插槽 session_summary，位置在 memory 之后、history 之前；固定顺序：platform → soul → memory(core+recall) → knowledge → tools → session_summary → history → current。
- 原始消息永不删除，界面照常显示全部。

## 2. 长期记忆写入
- 抽取输入：当前用户消息 + 上一条助手回复 + 最近 2 轮（仅作理解指代的上下文），只抽用户亲口陈述的事实。
- 去重候选：不再给最近 200 条，改为“同 subject_key 的条目 + 向量最相似 top-20（同 scope 可见范围）”。
- 操作集：ADD / UPDATE / DELETE / NOOP。DELETE 用于用户说“忘掉…/不再是…”，置 status=deleted、valid_to=now，写事件；仍不覆盖 user_edited（UPDATE 遇 user_edited 改为 NOOP 并记 reason）。
- DELETE 接口和抽取 DELETE 在删除事务中按记忆 ID 幂等入队 `rewrite_summary`。通过 conversation_id 或 source_ref 查找同一所有者的来源会话；找不到会话则跳过。任务从执行时的最新摘要移除事实，追加版本并保持覆盖范围和消息数，旧摘要及原始消息不删除。无摘要时写跳过事件；不遍历其他会话或重新摘要历史。停用 Agent 或关闭记忆不阻止已入队的遗忘清理。
- 用户显式“记住…”：importance≥8，可 pinned。
- kind 启用：fact / preference / episode（带时间的事件）/ procedure（用户希望的做事方式）。
- 写入来源抽象：所有写入都经 Memory Service，source_type 字段区分 chat_extraction / user_edit / tool / import / delegation（后三种先留枚举与接口，tool 等工具模块上线后接 remember/forget 工具）。
- sensitive 标记：身份证号、密码、银行卡等不入库（抽取 prompt + 写入前正则兜底），健康等敏感信息 sensitive=true，默认不进 core 视图只参与召回。

## 3. 召回
- 两阶段：pgvector HNSW 取候选 top-50（先确认 pgvector 版本；≥0.8 用 hnsw.iterative_scan 处理过滤；否则加大 ef_search 并在 SQL 过滤），再精排。
- 精排分 = 0.6×相关性 + 0.2×importance/10 + 0.2×retention（§4）；低于 min_relevance（默认 0.35）不注入；与 core 视图已含条目去重。
- 查询文本 = 当前消息 + 上一条用户消息。
- 被召回的条目：更新 last_used_at，并强化（§4）。

## 4. 遗忘与巩固
- retention = exp(-Δt / strength_days)，Δt 从 last_used_at（无则 created_at）算起；pinned 与 user_edited 不衰减。
- 被召回或被用户确认一次：strength_days = min(strength_days×1.5, 365)。
- 每日维护任务：retention<0.1 且 importance≤5 且未 pinned → status=archived（新增状态，事件 ARCHIVE）；archived 不进上下文，可在界面恢复（RESTORE）。
- 每日巩固：同用户同 namespace 余弦>0.92 的 active 条目，用 flash 判断是否同一事实，是则合并为新条目并 supersede 旧条目（事件 MERGE）。
- 时间可注入（Clock 接口），测试里可快进。

## 5. 任务队列可靠性
- memory_tasks 泛化为 memory_jobs(id, kind: extract|summarize|maintain|rewrite_summary, ref, owner_user_id, status, attempts, next_run_at, last_error, created_at, completed_at)，幂等键 (kind, ref)。rewrite_summary 的 ref 为被删除记忆 ID，复用退避、dead 和手动 retry；事件为 SUMMARY_REWRITE_QUEUED、SUMMARY_REWRITTEN、SUMMARY_REWRITE_SKIPPED、REWRITE_SUMMARY_FAILED。
- 失败指数退避重试（30s/2m/10m），3 次后 dead；保留 EXTRACT_FAILED 等事件。worker 多实例安全（SKIP LOCKED + 同用户 advisory lock）。
- 每日维护任务由 worker 定时入队。

## 6. 配置（memory_policy 扩展，旧字段兼容，带默认值与校验）
enabled, core_budget_tokens(800), recall_top_k(5), min_relevance(0.35), history_budget_tokens(6000), summary_trigger_tokens(3000), summary_budget_tokens(800), decay_enabled(true)。

## 7. 可观测性（给后续 runs 页面用）
- 每条助手消息的 context_segments 增加明细：memory_core 条目 id、recall 条目 id+分数、summary id 及版本、history 起止消息 id、各段 token 估算。
- `history_budget` 审计段记录 `over_budget`、`truncated`、`dropped_rounds`、`truncated_message_ids` 和保留历史起止 ID；此段不注入模型，estimated_tokens=0，实际历史成本仍在 history 段中。向量候选检索查询独立限制为 6000 估算 token，聊天和抽取模型中的当前消息仍完整。
- 新接口 GET /api/v1/memory/stats：各状态/namespace 数量、待处理/失败/dead 任务数、最近一次摘要与维护时间。

## 8. API（全部保持现有 6 个接口向后兼容，只新增）
- GET /api/v1/memories 增加 cursor 分页（limit 默认 50 最大 200，返回 next_cursor），status 支持 archived；scope=all_visible&agent_id= 一次返回 user 层+该 Agent 层。
- POST /api/v1/memories/{id}/restore（archived/deleted → active，事件 RESTORE）。
- POST /api/v1/memories  手动新增（source=user_edited）。
- GET /api/v1/conversations/{id}/summary 最新摘要及覆盖范围。
- GET /api/v1/memory/stats；POST /api/v1/memory/jobs/{id}/retry（仅所有者，dead→pending）。
- 输出完整 API 文档，入库到 docs/api/memory.md（含 JSON 示例），不再只放 .output。

## 9. 扩展接入点（只定义接口与文档，不实现）
- Store / Extractor / Retriever / Summarizer 接口，便于换模型或换存储。
- 多 Agent 共享：后续加 memory_shares(agent_id → 可读 scope/namespace) 授权表，召回时按授权扩大可见范围；本次在文档写清接法。
- 委派：子 Agent 产出的结论以 source_type=delegation 写入委托方 agent 层。
- 评测：§10 的场景做成可重复运行的评测集（make memory-eval），输出命中率与 token 统计，后续毕设评估直接用。

## 10. 验收（真实 PG + 真实百炼）
1. 长对话：单会话 120+ 条消息，生成多版摘要；每轮 prompt 估算 token 不超过 各预算之和；第 5 条说的事实在第 120 条仍能答对（摘要或长期记忆任一命中）。
2. 更新：雅思目标 6.5 → 7，旧条目 superseded，回答用新值。
3. 遗忘指令：“忘掉我在准备雅思” → deleted，之后不再注入。
4. user_edited 不被自动覆盖。
5. 衰减：时钟快进 200 天，低重要度条目被 archived，pinned 不受影响；restore 后可再召回。
6. 巩固：两条近似重复合并成一条。
7. 隔离：user 层所有 Agent 可见，agent 层仅本 Agent；别的用户和管理员看不到。
8. 失败重试：模拟模型失败，任务按退避重试，3 次后 dead，retry 接口可恢复。
9. 召回阈值：无关问题不注入记忆。
10. 现有 6 个接口行为不变（前端已在用）。
11. 真实项目长文超过历史预算后仍正常回复，历史具有 over_budget/truncated 明细，当前原文完整；删除已进入摘要的记忆，幂等入队并生成不含该事实的新摘要版本；超模型上下文返回 422。
检查：gofmt、go vet/test/build、make migrate、make integration 全过；make up 后服务正常；diff 无 Key。
遗忘排序、归档和维护合并已包含在记忆 v2；后续安排为多 Agent 共享授权与评测扩展，见 [路线图](ROADMAP.md)。

## 实现说明（S1 收尾）

当前实现新增 007/008/009 迁移：扩展 memory_items 状态、来源、保留率字段，建立 conversation_summaries、memory_jobs 和合并来源关系，并允许摘要重写任务；历史窗口按最旧轮丢弃、最后一轮截断逐级降级。qwen3.8-flash 关闭思考后将旧摘要和新增消息滚动合并，Context Builder 通过 session_summary 注入最新版本。召回使用 pgvector 0.8.6 的 HNSW 候选 50 条，再按相关性、importance 和 retention 精排并应用 min_relevance。worker 前两次失败分别等待 30 秒、2 分钟，第三次失败进入 dead（保留 10 分钟 next_run_at，但不会自动领取）；每日维护计算 retention、归档低价值条目，并将同一授权范围内模型确认的重复事实合并为新版本。

为保持旧客户端行为，不带分页参数的旧列表请求仍返回完整列表；显式分页和 all_visible 查询默认每页 50 条。完整接口说明见 [记忆 API](api/memory.md)。

扩展接口预留在 Memory Service：Store、Extractor、Retriever、Summarizer 可拆分为独立实现。多 Agent 共享应增加 memory_shares 授权表，在 visible 查询中按授权 scope 扩大范围；委派结果通过 source=delegation 写入委托方 agent 层；评测由 `make memory-eval` 使用真实 PostgreSQL 和百炼运行。
