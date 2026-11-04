# xa 产品路线图

## 产品定位

xa 是通用的个人 Agent 平台。用户注册后自动拥有一个默认主 Agent，也可以创建多个专职 Agent；公共 Agent 由管理员维护。Agent 的人格保存在随版本发布的 `soul.md`，长期记忆以结构化的 `memory_items` 存储在 PostgreSQL 与 pgvector 中，并通过 `memory.md` 提供可查看、可编辑的展示视图。

记忆分为用户共享层、Agent 私有层和会话层。非主 Agent 写入用户共享层时，内容必须先进入候选状态。遗忘曲线只影响召回排序，不物理删除记忆。知识库是独立的 RAG，通过 pgvector HNSW 与关键词检索，再使用 RRF 融合，由 `kb_search` 工具提供给 Agent。

多 Agent 只支持主 Agent 委派专职 Agent，委派深度不超过 2，不实现 Agent 之间的自由群聊。模型供应商保留统一接口，当前只接入阿里百炼的 OpenAI 兼容接口。管理端重点展示可观测性，包括每轮对话的上下文组装、召回片段、工具调用、记忆写入、token 用量和耗时。英语口语教练只是平台上的一个 Agent，配合知识库、评分和 FSRS 错题复习实现，不单独建立另一套系统。

## 暂不做

- 多租户。
- Realtime 语音。
- MCP。
- 模型与评测后台。
- Flutter 移动端；是否接入在 Web 验收后根据毕设要求决定。
- Agent 之间的自由群聊，以及超过两层的委派链。

## 演示场景

- **S1 默认 Agent 记忆**：默认 Agent 跨会话记得用户，用户可以查看和编辑 `memory.md`。
- **S2 口语教练**：新建口语教练 Agent，上传资料后检索出题、打分，并记住用户常犯错误。
- **S3 运行记录**：管理端查看一轮对话的完整运行记录。
- **S4 知识库质量**：查看知识库切片，执行检索测试，并在小评测集上计算 recall@5 和 MRR。
- **S5 主 Agent 委派**：主 Agent 委派口语教练完成专职任务。
- **S6 记忆评测**：参照 LongMemEval 的能力分类自建评测集，覆盖隔离和投毒测试。

## 阶段计划

| 阶段 | 工作内容 | 验收重点 |
| --- | --- | --- |
| 阶段一：清理 | 清理文档、规范和占位事项，建立本路线图。 | 方向、冻结范围和后续顺序一致；本阶段不删除代码。 |
| 阶段二：对话基础 | 接入阿里百炼，完成 Eino 对话循环、SSE、会话与消息存储。 | 真实模型可以完成一轮文本对话，消息、工具事件和失败状态可追踪。 |
| 阶段三：记忆 | 建立 `memory_items`，实现记忆抽取、候选审核、作用域召回和 `memory.md` 视图。 | 默认 Agent 跨会话记忆可查看、编辑，用户共享层和 Agent 私有层隔离。 |
| 阶段四：运行观测 | 实现 trace 页和每轮对话的运行记录。 | 可查看上下文组装、召回片段、工具调用、记忆写入、token 和耗时。 |
| 阶段五：知识库 RAG | 完成文档摄取、切片、pgvector HNSW、关键词检索、RRF 融合和 `kb_search`。 | 可查看切片，执行检索测试并计算 recall@5、MRR。 |
| 阶段六：口语教练与 FSRS | 以配置创建口语教练 Agent，接入评分、知识库出题和 FSRS 错题复习。 | 完成上传资料、检索出题、评分、错题记忆和复习闭环。 |
| 阶段七：委派 | 实现主 Agent 委派专职 Agent，限制委派深度不超过 2。 | 主 Agent 能调用口语教练，禁止自由群聊和无限递归。 |
| 阶段八：记忆质量 | 增加遗忘曲线召回排序和记忆评测集。 | 完成隔离、投毒等测试，遗忘只改变排序，不物理删除记忆。 |

## Icebox

以下条目来自当前仓库的占位扫描。后端条目是只有接口或数据声明、没有可运行实现的包，或包含 `panic("TODO")` 的包；前端条目是菜单、路由页面或页面内仍显示“待开发”的功能。本阶段只登记，不删除任何代码。仅有普通数据结构且没有接口或 TODO 的 contract/model 包不单列。

### 后端占位包

| 原路径 | 原本打算做什么 | 再启用阶段 |
| --- | --- | --- |
| `server/internal/agent/binding` | 保存 Agent 版本与工具、知识库、评价能力的绑定快照。 | 阶段二；知识库和评价绑定分别在阶段五、六补全。 |
| `server/internal/agent/conversation` | 保存会话、轮次、消息并控制同一会话的运行通道。 | 阶段二。 |
| `server/internal/agent/memory` | 提供长期记忆的作用域查询和写入接口。 | 阶段三。 |
| `server/internal/agent/prompt` | 解析版本化人格和提示词槽位，生成运行快照。 | 阶段二。 |
| `server/internal/agent/runtime` | 执行 Agent 循环并把模型事件转换为平台事件；当前含 `panic("TODO: implement agent runner")`。 | 阶段二。 |
| `server/internal/agent/runtime/eino` | 创建或复用 Eino ChatModelAgent；当前含 `panic("TODO: build or reuse Eino ChatModelAgent")`。 | 阶段二。 |
| `server/internal/agent/transport/sse` | 将 Agent 事件编码为 SSE 流。 | 阶段二。 |
| `server/internal/agent/transport/websocket` | 提供语音会话的 WebSocket 生命周期。 | 冻结；只有毕设重新需要实时语音时再评估。 |
| `server/internal/evaluation/review` | 提供人工复核和评分结果查询。 | 阶段六。 |
| `server/internal/evaluation/rubric` | 管理口语教练的评价规则和版本。 | 阶段六。 |
| `server/internal/evaluation/scoring` | 执行语法、发音和综合评分并保留证据。 | 阶段六。 |
| `server/internal/framework/observability` | 定义日志、指标和 trace 的统一接口。 | 阶段四。 |
| `server/internal/framework/task` | 定义异步任务队列和幂等任务边界。 | 阶段三；阶段六扩展评价任务。 |
| `server/internal/infra/embedding` | 为知识库切片和记忆生成向量。 | 阶段五。 |
| `server/internal/infra/model` | 提供统一文本模型接口并适配供应商。 | 阶段二。 |
| `server/internal/infra/provider` | 根据能力和配置选择模型供应商适配器。 | 阶段二。 |
| `server/internal/infra/realtime` | 提供供应商无关的实时连接。 | 冻结；只有毕设重新需要实时语音时再评估。 |
| `server/internal/infra/rerank` | 为检索候选提供重排序能力。 | 暂不排期；阶段五先使用 HNSW、关键词检索和 RRF，指标证明需要时再评估。 |
| `server/internal/infra/speech` | 提供语音识别和语音合成。 | 冻结；只有毕设重新需要实时语音时再评估。 |
| `server/internal/rag` | 定义 Agent 访问知识库的检索契约。 | 阶段五。 |
| `server/internal/rag/ingestion` | 解析文档、分块、生成向量并写入索引。 | 阶段五。 |
| `server/internal/rag/knowledge` | 保存知识库、文档和切片元数据。 | 阶段五。 |
| `server/internal/rag/retrieval` | 实现向量与关键词检索、RRF 融合和检索记录。 | 阶段五。 |
| `server/internal/system/audit` | 写入管理操作和关键运行操作的审计事件。 | 阶段四。 |
| `server/internal/system` | 为业务模块提供工作空间资源归属和统一权限校验契约。 | 阶段二。 |
| `server/internal/system/permission` | 判定用户对 Agent、工具、模型和知识库的资源权限。 | 阶段二。 |
| `server/internal/task/dispatcher` | 领取、重试和确认异步任务。 | 阶段三。 |
| `server/internal/task/handler` | 为记忆、知识摄取和评价任务提供幂等处理器。 | 阶段三；阶段五、六增加对应处理器。 |
| `server/internal/tool/catalog` | 校验工具版本、Schema 和权限，生成运行目录。 | 阶段二。 |
| `server/internal/tool/mcp` | 连接 MCP 服务并发现、调用远程工具。 | 冻结；只有 MCP 重新进入产品范围时再评估。 |
| `server/internal/tool/runtime` | 执行已授权工具并处理超时、取消和结构化结果。 | 阶段二。 |

### 前端占位页面和菜单

| 原路径 | 原本打算做什么 | 再启用阶段 |
| --- | --- | --- |
| `web/config/modules.ts#/models`、`web/src/pages/module/index.tsx` | 管理模型配置、供应商、能力和连接状态。 | 暂不排期；阶段二只通过配置接入阿里百炼，后台模型管理待后续重新确认。 |
| `web/config/modules.ts#/knowledge`、`web/src/pages/module/index.tsx` | 管理知识库、文档、切片和检索验证。 | 阶段五。 |
| `web/config/modules.ts#/tools`、`web/src/pages/module/index.tsx` | 管理内置工具目录和工具权限；其中 MCP 子页原本用于 MCP 服务。 | 阶段二实现内置工具；MCP 子页冻结。 |
| `web/config/modules.ts#/evaluation`、`web/src/pages/module/index.tsx` | 管理评价规则、验证输入和评分依据。 | 阶段六；评价后台在此前保持 Icebox。 |
| `web/config/modules.ts#/tasks`、`web/src/pages/module/index.tsx` | 查看异步任务、处理器、失败原因和重试状态。 | 阶段三。 |
| `web/config/modules.ts#/users`、`web/src/pages/module/index.tsx` | 管理账号、角色、启停状态和资源授权。 | 账号基础能力完成后再评估管理端批次。 |
| `web/config/modules.ts#/operations`、`web/src/pages/module/index.tsx` | 查看运行调用、错误、资源用量和审计记录。 | 阶段四 trace 页。 |
| `web-user/src/pages/module/index.tsx#/history` | 查看、搜索、继续和删除个人会话历史。 | 阶段二。 |
| `web-user/src/pages/module/index.tsx#/memory` | 按 Agent 查看、修改、删除记忆并控制记忆使用。 | 阶段三。 |
| `web-user/src/pages/module/index.tsx#/feedback` | 查看评分、纠错、评价依据以及关联对话和录音。 | 阶段六。 |
| `web-user/src/pages/module/index.tsx#/settings` | 提供资料修改、邮箱验证和密码找回等账号安全设置。 | 阶段二后的账号完善批次。 |
| `web-user/src/pages/agents/index.tsx#/explore`、`#/my-agents` | 在 Agent 目录中显示文本对话能力，并配置模型、工具和知识库绑定。 | 阶段二接入文本对话；阶段五补知识库绑定。 |
| `web-user/src/pages/chat/index.tsx#/chat` | 提供会话列表、文本消息、SSE 回复、实时语音和 Agent 能力详情。 | 阶段二接入会话与 SSE；Realtime 语音冻结。 |
