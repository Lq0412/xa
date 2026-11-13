# 知识库检索与 Agent 接入（R2）

R2 在 R1 摄取数据上提供按需检索和运行快照。继续使用同一 PostgreSQL、1024 维 `qwen3.7-text-embedding` 和 NativeEngine；没有新绑定表、检索页面或独立进程。

## 绑定与权限

通过已有的 `POST /api/v1/agents` 或 `PUT /api/v1/agents/{agent_id}/config` 保存完整配置。绑定形态保持不变：

```json
{"knowledge":[{"knowledge_base_id":"kb_…","enabled":true,"filter":""}]}
```

私有知识库只能绑定所有者自己的私有 Agent；公共知识库只能绑定公共 Agent，创建和管理仍要求管理员。保存时检查所有绑定，包括禁用条目；重复 ID、不可访问的库、可见性不匹配以及非空 `filter` 返回 `422 invalid_agent`，错误体继续为 `{code,message,request_id}`。`filter` 仍为原有字节字段，不支持过滤表达式。

新运行使用当前 Agent 版本，已有运行和审批恢复继续使用其固定的不可变版本。每次 `kb_search` 都读取该版本，重新按调用者权限、知识库可见性及删除状态计算范围；管理员没有查看他人私有库的特权。模型提供的 `collection_ids` 仅取交集，不能扩大范围。未绑定、禁用或没有可读 ready 文档时返回空结果。

## 检索过程

1. 查询最长 200 字，`top_k` 默认 5、上限 20。
2. 用已有百炼批量客户端生成查询向量，传 `text_type=query`；正文摄取仍使用 `document`。
3. 向量候选按 cosine 距离排序；中文关键词沿用 gse 0.80.3 的中文搜索分词，使用 PostgreSQL `simple` 全文索引，查询词以 OR 连接。两路都在取候选前限制权限、绑定、ready 状态和 embedding 模型。
4. 查询事务内设置 `hnsw.iterative_scan=strict_order`。环境已验证 pgvector 0.8.7；运行要求支持迭代扫描的 0.8 及以上版本。
5. 对两路名次使用 RRF：`Σ 1 / (k + rank)`，名次从 1 开始。相同切片只返回一次，并列按切片 ID 排序。
6. 返回内容受总字符预算限制；最后一条被截短时标记 `truncated`，并更新该片段的字符终点与估算 token 数。

| 配置 | 默认值 | 含义 |
| --- | --- | --- |
| `XA_KB_SEARCH_CANDIDATES` | 50 | 每路最多候选数，范围 20–1000 |
| `XA_KB_SEARCH_MAX_CHARS` | 6000 | 返回片段正文的总 Unicode 字符上限，范围 1–50000 |
| `XA_KB_SEARCH_MIN_SIMILARITY` | 0.3 | 向量候选的最小 cosine 相似度，范围 -1–1；关键词命中不受此门槛限制 |
| `XA_KB_RRF_CONSTANT` | 60 | RRF 排名常数，至少为 1 |

相似度门槛是初始配置，不代表已完成质量校准；关键词命中也不保证片段足够回答问题。R3 使用评测集确定质量与参数。HNSW 迭代扫描遵循 [pgvector 文档](https://github.com/pgvector/pgvector#iterative-index-scans)，RRF 计算与默认常数参考 [Elasticsearch 文档](https://www.elastic.co/docs/reference/elasticsearch/rest-apis/reciprocal-rank-fusion)。

## 工具和追溯

`kb_search` 已可用，默认在存在启用绑定时进入模型工具目录，也可以在 `configuration.tools[]` 中显式关闭。没有知识绑定时，即使显式启用工具，也会被 `knowledge_unbound` 条件排除。没有每轮预取。

工具结果包含查询、运行使用的 Agent 版本、检索时间和命中数组。每项包括来源 ID、文档和切片 ID、文档标题、正文、标题路径、页码、字符范围、原文 SHA-256、切片版本、embedding 模型、两路名次与 RRF 分数。资料是外部数据，工具说明明确禁止将其当作系统指令，并要求资料不足时如实说明。

现有工具循环将结果写入 `tool_results.data`，通过 `GET /api/v1/runs/{run_id}/tool-results/{call_id}` 查询。`run_id + call_id + source_id` 标识当次来源；删除原文或更新绑定不会改写已经保存的结果。历史快照遵守已有运行记录保留期。R3 再把这些来源映射为 `[S1]` 和可点击引用。

## 验收方式

准备真实 PostgreSQL、Redis、RustFS、百炼和 Poppler，按 [启动说明](startup.md) 注入运行配置。在 `server/` 执行：

```bash
go test -race -tags=integration ./internal/rag -run TestKnowledgeR2 -v -count=1
```

测试创建独立数据库和对象前缀，启动生产 server，调用 Huma 接口保存绑定，并用真实百炼完成摄取、查询和对话工具循环；结束后清理测试资源。覆盖绑定权限、中文专名与编号、同义改写、无命中、参数不能扩权、并发检索、片段预算、版本变更、删除与历史快照。

2026-11-13 实测通过：上述 R2 race 集成测试用时约 49 秒；私有 Agent 的 SSE v1 和公共 Agent 的 SSE v2 均产生成功的 `kb_search` 记录。编号、中文专名和同义问法均将预期段落排在首位，两路名次为 1、RRF 分数约 0.03279。无关英文查询返回空结果。旧版本中预存的越权绑定也无法读取私有资料。删除并清理原文后，已有工具结果仍能查询原命中内容。

`make check` 已通过 Go 格式、test/vet/build、接口生成一致性和路由检查，以及两个前端的 lint/TypeScript/build。全部带 integration 标签的测试已编译通过；本期没有执行会迁移日常数据库的全量 `make integration`，真实依赖验收使用上述独立数据库测试。

R1 的 `TestKnowledgeR1` race 回归也通过（约 46 秒），覆盖权限、配额、去重、重试、中文文本和 PDF、扫描件失败、切片出处、原文清理及真实 worker。此结果属于开发验证，R1、R2 仍等待独立验收。

人工接口验收：创建库并上传 Markdown → 等待 ready → 用原有配置保存接口绑定到自己的 Agent → 通过已有对话接口提问 → 在运行详情查询 `kb_search` 的结构化结果 → 删除文档后重新查询，确认没有新命中且旧结果仍可核对。完整请求结构以 [生成接口](api/openapi.yaml) 为准；网页创建、上传和绑定入口在 R3 提供。

## 边界与取舍

- R2 不新增检索 HTTP 路由。对话使用已有 `kb_search` 工具；R3 的检索比较界面与评测入口另行实现。
- 不做 rerank、自动预取、引用界面、OCR、父子切片、用户间共享或公共 Agent 叠加私人资料。
- 不改变记忆表、任务种类、SSE 帧格式或已有运行记录保留策略。
- R1 尚待独立验收合并，R2 PR 暂以 R1 分支为基线；开发自查和自动测试不代替独立验收。
