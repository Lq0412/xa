# Agent 管理与运行平台架构设计 v2

## 1. 产品定位

系统是一个可视化 Agent 管理与运行平台。管理员可以创建多个 Agent，分别配置提示词、模型、工具、知识库、评价规则、记忆策略和使用权限；用户在主对话页选择一个已发布 Agent 进行对话。

每个可创建、可发布、可使用的学习任务都是一个 Agent。例如：

~~~
IELTS 口语教练
英文面试教练
商务英语教练
发音纠错教练
每日单词教练
自由英语陪练
~~~

这些名称只是配置，不对应不同的代码模块。“场景”只作为名称、用途和标签存在，不设计固定的 scenario 业务分支。

“主 Agent”如果需要存在，只是设置了 is_default 的普通 Agent，不使用特殊运行逻辑。

~~~
Agent 配置
    ↓
Agent 发布版本
    ↓
统一 Agent Runtime
    ↓
模型、工具、知识库、评价和记忆
~~~

## 2. 设计原则

- 配置驱动：新增 Agent 只增加配置，不复制运行代码。
- 版本不可变：发布版本不能修改，会话固定绑定版本。
- 记忆隔离：记忆默认按 workspace_id + user_id + agent_id 隔离。
- 管理面与运行面分离：管理面编辑和发布，运行面只使用已发布版本。
- 高内聚、低耦合：模块拥有自己的领域对象、仓储和用例，不跨模块访问数据库表。
- 少量核心机制：首期不引入固定场景模块、特殊主 Agent、全局 Agent 单例和独立事件总线。

## 3. 系统边界

~~~mermaid
flowchart LR
    Admin[管理后台] --> Server[server]
    Client[用户客户端] --> Server

    Server --> System[system]
    Server --> Agent[agent]
    Agent --> Runtime[Agent Runtime]
    Runtime --> Eino[Eino]
    Runtime --> Infra[infra]
    Runtime --> Tool[tool]
    Runtime --> Rag[rag]
    Runtime --> Evaluation[evaluation]

    Worker[worker] --> Agent
    Worker --> Rag
    Worker --> Evaluation

    Agent --> PostgreSQL[(PostgreSQL)]
    Agent --> Redis[(Redis)]
    Agent --> ObjectStorage[(对象存储)]
~~~

系统负责：

- Agent 配置、发布和权限。
- 文本对话和语音对话。
- 工具调用和知识检索。
- 评价、评分、会话历史和 Agent 记忆。
- 异步任务、审计和运行监控。

## 4. 进程划分

项目使用一个 Go module，包含两个主要进程：

| 进程 | 职责 |
| --- | --- |
| server | 管理接口、用户接口、SSE、WebSocket 和 Agent 运行请求 |
| worker | 评价、摘要、记忆提取和知识摄取等异步任务 |

入口：

~~~
cmd/server/main.go
cmd/worker/main.go
~~~

server 和 worker 共用业务模块，由各自的 bootstrap 负责组装依赖。

## 5. 目录结构

~~~text
server/
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── worker/
│       └── main.go
│
├── internal/
│   ├── bootstrap/
│   │   ├── server.go
│   │   └── worker.go
│   │
│   ├── framework/
│   │   ├── config/
│   │   ├── http/
│   │   ├── auth/
│   │   ├── postgres/
│   │   ├── redis/
│   │   ├── storage/
│   │   ├── task/
│   │   └── observability/
│   │
│   ├── system/
│   │   ├── user/
│   │   ├── permission/
│   │   ├── audit/
│   │   └── contract.go
│   │
│   ├── agent/
│   │   ├── definition/
│   │   ├── prompt/
│   │   ├── binding/
│   │   ├── runtime/
│   │   │   ├── engine.go
│   │   │   ├── context.go
│   │   │   ├── event.go
│   │   │   └── eino/
│   │   ├── conversation/
│   │   ├── memory/
│   │   ├── transport/
│   │   │   ├── http/
│   │   │   ├── sse/
│   │   │   └── websocket/
│   │   └── contract.go
│   │
│   ├── tool/
│   │   ├── catalog/
│   │   ├── runtime/
│   │   ├── mcp/
│   │   └── contract.go
│   │
│   ├── rag/
│   │   ├── knowledge/
│   │   ├── ingestion/
│   │   ├── retrieval/
│   │   └── contract.go
│   │
│   ├── evaluation/
│   │   ├── rubric/
│   │   ├── scoring/
│   │   ├── review/
│   │   └── contract.go
│   │
│   ├── infra/
│   │   ├── model/
│   │   ├── realtime/
│   │   ├── speech/
│   │   ├── embedding/
│   │   ├── rerank/
│   │   └── provider/
│   │
│   └── task/
│       ├── definition/
│       ├── dispatcher/
│       └── handler/
│
├── migrations/
├── configs/
├── api/
│   └── openapi.yaml
├── docs/
├── go.mod
└── go.sum
~~~

模块内部按实际复杂度组织：

~~~text
internal/agent/definition/
├── model.go
├── service.go
├── repository.go
└── store/
~~~

- model.go：领域对象和状态规则。
- service.go：用例。
- repository.go：当前包需要的持久化接口。
- store/：数据库行对象和查询实现。
- transport/：协议适配，不把 HTTP 请求对象传入领域层。

不建立全局 common model 包，也不让数据库行对象跨模块流动。

## 6. 模块职责

### 6.1 framework

提供配置、HTTP、认证上下文、PostgreSQL、Redis、对象存储、任务基础设施、日志、指标、追踪和优雅关闭能力。它不定义 Agent、会话、评分和知识库等业务对象。

### 6.2 system

负责用户、登录、角色、权限、工作空间和审计。其他模块通过接口校验身份和资源权限，不直接读取用户表。

### 6.3 agent

负责 Agent 定义、草稿、版本、提示词、绑定、运行、会话、消息、记忆和对外接口。它不直接创建模型客户端，也不直接连接具体模型供应商。

### 6.4 tool

负责工具目录、工具版本、参数 Schema、注册、权限、超时、取消、结果转换和 MCP 客户端。具体业务工具由所属模块提供，再注册到工具运行时。

### 6.5 rag

负责知识库、文档、解析、分块、向量生成、检索、过滤、重排序和检索记录。Agent 只通过检索接口获取知识。

### 6.6 evaluation

负责评价规则、评分维度、语法评价、发音评价、综合评分、评价证据和评价结果。评分结果必须保留评分依据。

### 6.7 infra

负责对话模型、Realtime 模型、语音识别、语音合成、向量模型、重排序模型、供应商路由、超时、取消、错误转换和调用指标。它不包含 Agent 提示词、学习场景和评分规则。

infra 采用“预先适配、统一调用、配置选择”的方式管理模型供应商：

- 代码中为每种能力注册供应商适配器，集中处理鉴权、请求格式、流式事件和错误差异。
- 配置中登记供应商、模型和能力，Agent 版本只保存模型绑定，不保存供应商调用细节。
- 运行时通过能力接口选择适配器，Agent 不直接依赖具体供应商 SDK。

文本模型、Realtime 模型、Embedding、语音和重排序分别使用独立能力接口。Realtime 会话是有状态的连接实例，不能复用文本模型的单轮调用接口；上游事件必须先转换成平台统一事件。

## 7. 核心领域模型

### 7.1 AgentDefinition

表示 Agent 的稳定身份：

~~~text
id
workspace_id
name
avatar
description
tags
visibility
status
is_default
created_by
created_at
updated_at
~~~

is_default 只表示主对话页的默认选择。

### 7.2 AgentVersion

表示一次可运行配置：

~~~text
id
agent_id
version_number
status
prompt_snapshot
model_snapshot
memory_policy
runtime_limits
model_bindings
config_checksum
created_by
published_at
created_at
~~~

状态：

~~~text
draft
published
offline
archived
~~~

发布后的版本不可修改。

### 7.3 模型供应商和模型配置

模型配置分为三层：

~~~text
ModelProvider
    供应商身份、协议类型、端点和凭证引用

ModelProfile
    模型标识、能力、上下文限制、参数和版本

AgentVersionModelBinding
    Agent 版本在 text、realtime、embedding、speech、rerank 等能力上使用的模型
~~~

`ModelProvider` 和 `ModelProfile` 是平台可管理的注册信息，`AgentVersionModelBinding` 在发布时固化到版本快照。凭证只保存引用，不把密钥写入 Agent 配置或日志。

### 7.4 AgentPrompt

提示词采用槽位管理：

~~~text
system
behavior
task
safety
output_format
first_message
~~~

每个槽位包含内容、启用状态、顺序、变量和作用范围。解析器负责校验变量、合并槽位和生成提示词指纹。

### 7.5 AgentConversation

~~~text
id
workspace_id
user_id
agent_id
agent_version_id
title
status
created_at
updated_at
~~~

会话创建后固定 agent_version_id。切换 Agent 时创建新会话。

### 7.6 AgentMemory

~~~text
id
workspace_id
user_id
agent_id
namespace
memory_type
content
source
confidence
status
created_at
updated_at
expires_at
~~~

默认作用域为 workspace_id + user_id + agent_id + namespace。

## 8. Agent 管理流程

~~~mermaid
flowchart LR
    Create[创建 Agent] --> Draft[编辑草稿]
    Draft --> Validate[校验配置]
    Validate --> Preview[预览运行]
    Preview --> Publish[发布版本]
    Publish --> Online[用户可使用]
    Online --> NewDraft[创建新草稿]
    NewDraft --> Draft
    Online --> Offline[下线版本]
~~~

管理员配置名称、描述、头像、标签、提示词、模型、工具、知识库、评价规则、记忆策略和运行限制。

发布前校验提示词变量、模型、工具版本、工具 Schema、知识库、评价规则、记忆策略和运行限制。配置错误直接返回，不能使用静默默认值。

预览使用草稿版本，但不写入正式会话和长期记忆。有副作用的工具默认禁止调用，预览日志与正式日志分开。

发布过程在事务中完成：

~~~text
校验草稿
→ 生成不可变版本
→ 保存绑定快照
→ 计算配置指纹
→ 更新当前发布版本
→ 写入审计记录
~~~

发布新版本后，新会话使用新版本，旧会话继续使用原版本。

## 9. Agent 运行流程

~~~mermaid
sequenceDiagram
    participant C as 客户端
    participant S as server
    participant A as agent
    participant M as memory
    participant R as rag
    participant T as tool
    participant E as Eino
    participant L as 模型
    participant W as worker

    C->>S: 发送消息
    S->>A: SendTurn
    A->>A: 加载会话和 AgentVersion
    A->>A: PromptResolver 合并提示词
    A->>A: ToolCatalog 解析工具
    A->>A: ReActAgentProvider 获取运行实例
    A->>M: 读取 Agent 记忆
    A->>R: 执行知识检索
    A->>E: 创建或复用运行实例
    E->>L: 请求模型
    L-->>E: 文本增量或工具调用
    E->>T: 执行工具
    T-->>E: 返回结构化结果
    E-->>S: 流式事件
    S-->>C: SSE 或 WebSocket
    A->>A: 保存消息和轮次
    A->>W: 创建评价和记忆任务
~~~

一次请求必须携带：

~~~text
workspace_id
user_id
agent_id
agent_version_id
conversation_id
turn_id
request_id
~~~

## 10. ReAct Agent 和 Eino

系统对外只有一种 Agent：配置驱动的 ReAct Agent。用户不需要选择 chat、tool_calling 或 workflow 等运行模式。

一次运行由模型决定是否调用工具：

~~~text
读取 AgentVersion 和会话上下文
→ 调用模型
→ 模型直接回复，或请求调用工具
→ 执行已绑定的工具
→ 将工具结果放回上下文
→ 继续调用模型
→ 输出最终回复
~~~

Eino 是 Agent 执行框架，负责模型调用、工具调用循环、流式事件、Middleware、Callback 和中断恢复。Eino 的 ChatModelAgent 内部处理 ReAct 循环，Runner 负责启动和消费一次运行。

本系统自己负责：

- AgentDefinition 和 AgentVersion。
- 提示词槽位和 PromptResolver。
- 工具目录和 ToolCatalog。
- 会话、消息和 AgentMemory。
- 权限、审计、评价和异步任务。
- 模型、工具和运行结果的持久化。

### 10.1 运行组件

Agent Runtime 按 Vault 的实现思路拆成四个职责：

~~~text
AgentVersion
    ↓
PromptResolver
    ↓
ToolCatalog
    ↓
ReActAgentProvider
    ↓
Eino ChatModelAgent + Runner
~~~

- PromptResolver：读取当前 Agent 版本的提示词槽位，校验变量并生成最终提示词。
- ToolCatalog：读取 Agent 版本绑定的工具，校验工具权限和版本，生成本次运行的工具目录。
- ReActAgentProvider：根据 Agent 版本、提示词指纹和工具目录指纹创建或复用 Eino Agent。
- Runner：为当前用户会话执行一次 ReAct 运行，并把 Eino 事件转换成系统事件。

Provider 的缓存键至少包含：

~~~text
agent_version_id
config_checksum
tool_catalog_checksum
model_revision
knowledge_revision
~~~

Agent 实例只保存不可变配置和运行组件，不能保存用户消息、用户记忆和会话状态。用户数据通过每次运行的 RuntimeContext 传入，会话状态由本系统持久化。

### 10.2 适配边界

Eino 类型只出现在以下适配包：

~~~text
internal/agent/runtime/eino/
internal/infra/model/
internal/infra/realtime/
internal/tool/runtime/
internal/rag/retrieval/
~~~

领域层使用本系统自己的接口：

~~~go
type AgentRunner interface {
    Run(ctx context.Context, input RunInput) (<-chan Event, error)
}
~~~

这样可以使用 Eino 实现 ReAct Agent，同时避免 Eino 类型扩散到 AgentDefinition、Conversation 和 Memory 等领域代码。

### 10.3 模型供应商适配

模型供应商采用 Vault 中的预适配模式：

~~~text
AgentVersionModelBinding
    ↓
ModelResolver
    ↓
ProviderRegistry
    ↓
供应商适配器
    ↓
统一能力接口
~~~

平台为每种能力定义独立接口：

~~~text
TextModelProvider
RealtimeProvider
EmbeddingProvider
SpeechProvider
RerankProvider
~~~

同一供应商可以实现多个能力适配器，也可以只支持其中一种能力。适配器负责供应商协议、鉴权、请求参数、流式事件和错误转换；Agent、会话和评价模块只使用统一能力接口。

新增 Agent、切换已注册模型、增加已支持供应商的新模型都只需要配置。遇到全新的供应商协议时，只增加一次对应能力适配器，完成注册后后续 Agent 仍然只通过配置使用。

文本通道使用 Eino 的模型接口执行 ReAct 循环。Realtime 通道使用独立的 `RealtimeProvider` 创建一条有状态实时会话，并将音频、转写、打断和回复事件转换为平台事件。两种通道共享 Agent 版本、工具权限、会话记忆和评价任务，但不共用同一个运行接口。

## 11. 工具和记忆

工具用于评分、发音分析、训练题检索、学习计划查询、历史记录查询、知识库搜索和用户进度统计。

Agent 负责理解意图、决定是否调用工具和组织最终回复；工具负责参数校验、权限校验、业务执行、结构化返回、超时和取消。

工具绑定至少包含：

~~~text
tool_version_id
enabled
timeout
max_calls
allowed_roles
configuration
~~~

短期上下文由当前会话消息、当前 Agent 提示词、允许的工具、允许的知识库和长期记忆摘要组成。

长期记忆由异步任务提取、去重和写入。默认按 workspace_id + user_id + agent_id + namespace 隔离，禁止跨 Agent 读取。

## 12. 模块依赖

~~~mermaid
flowchart TD
    Cmd[cmd] --> Bootstrap[bootstrap]
    Bootstrap --> Framework[framework]
    Bootstrap --> System[system]
    Bootstrap --> Infra[infra]
    Bootstrap --> Tool[tool]
    Bootstrap --> Rag[rag]
    Bootstrap --> Evaluation[evaluation]
    Bootstrap --> Agent[agent]
    Bootstrap --> Task[task]

    System --> Framework
    Infra --> Framework
    Tool --> Framework
    Rag --> Framework
    Evaluation --> Framework

    Agent --> Framework
    Agent --> System
    Agent --> Infra
    Agent --> Tool
    Agent --> Rag
    Agent --> Evaluation

    Task --> Framework
    Task --> Agent
    Task --> Rag
    Task --> Evaluation
~~~

约束：

- framework 不依赖业务模块。
- infra 不依赖 agent、rag 或 system。
- tool 不依赖 agent。
- rag 和 evaluation 不读取 agent 表。
- agent 不直接创建模型、语音或检索客户端。
- bootstrap 只负责依赖装配。
- 各模块不直接访问其他模块的数据库表。

## 13. 数据存储

### PostgreSQL

保存业务事实：

~~~text
users
workspaces
roles
audit_events

agent_definitions
agent_versions
agent_version_tools
agent_version_knowledge
agent_version_evaluations
agent_conversations
agent_turns
agent_messages
agent_memories

tool_definitions
tool_versions
model_providers
model_profiles
agent_version_model_bindings
knowledge_bases
knowledge_documents
knowledge_chunks
evaluation_rubrics
evaluation_results
async_tasks
~~~

agent_version 发布后不可修改，agent_conversation.agent_version_id 不允许自动变化。向量检索首期使用 PostgreSQL 的 pgvector，知识库原始文件和音频存储在对象存储。

### Redis

只保存流式连接状态、取消标记、并发锁、限流计数、运行缓存和短期数据。Redis 丢失不能破坏 PostgreSQL 中的业务事实。

### 对象存储

保存用户音频、语音文件和知识库原始文档，数据库保存对象键、媒体类型、大小、时长和校验值。

## 14. 接口和流式协议

### Agent 管理接口

~~~text
GET    /api/v1/agents
POST   /api/v1/agents
GET    /api/v1/agents/{agent_id}
PATCH  /api/v1/agents/{agent_id}
GET    /api/v1/agents/{agent_id}/draft
PUT    /api/v1/agents/{agent_id}/draft
POST   /api/v1/agents/{agent_id}/validate
POST   /api/v1/agents/{agent_id}/preview
POST   /api/v1/agents/{agent_id}/publish
POST   /api/v1/agents/{agent_id}/offline
POST   /api/v1/agents/{agent_id}/default
GET    /api/v1/agents/{agent_id}/versions
~~~

### 用户接口

~~~text
GET    /api/v1/available-agents
POST   /api/v1/conversations
GET    /api/v1/conversations
GET    /api/v1/conversations/{conversation_id}
POST   /api/v1/conversations/{conversation_id}/turns
GET    /api/v1/conversations/{conversation_id}/stream
WS     /api/v1/conversations/{conversation_id}/voice
~~~

### 流式事件

~~~json
{
  "id": "event_01",
  "type": "message.delta",
  "request_id": "request_01",
  "conversation_id": "conversation_01",
  "turn_id": "turn_01",
  "sequence": 12,
  "data": {},
  "occurred_at": "2026-11-02T10:00:00Z"
}
~~~

事件类型：

~~~text
turn.started
message.delta
tool.started
tool.completed
message.completed
evaluation.queued
turn.completed
turn.failed
~~~

## 15. 异步任务

异步任务包括语法评价、发音评价、综合评分、会话摘要、长期记忆提取、知识文档解析、文档分块和向量生成。

任务状态：

~~~text
pending
running
succeeded
failed
dead
~~~

任务必须包含幂等键。重复执行不能重复写入评价结果或记忆。

## 16. 安全和可靠性

- 所有资源查询带工作空间、用户和资源归属条件。
- Agent、工具和知识库权限由服务端重新校验。
- 密钥只从配置或环境变量读取。
- 文件上传限制大小、类型和时长。
- 模型、语音、检索和工具调用都设置超时并支持取消。
- 关键写入使用事务，任务使用幂等键。
- 不使用静默 fallback，错误由上层明确处理。
- 日志不得输出密码、令牌、完整音频和模型密钥。

## 17. 对抗性审查

| 风险 | 约束 |
| --- | --- |
| Agent 越来越多导致代码复制 | Agent 只增加配置，所有 Agent 共用 Runtime |
| 修改提示词影响历史会话 | 会话固定绑定不可变版本 |
| 不同 Agent 之间记忆污染 | 记忆键包含 agent_id |
| 模型调用不属于当前 Agent 的工具 | Runtime 只注册绑定的工具，并在工具侧再次校验 |
| 预览写入正式数据 | 预览使用独立运行标识，默认不写记忆 |
| 发布后缓存仍使用旧配置 | 缓存键包含版本和配置指纹 |
| worker 重试产生重复结果 | 任务和结果使用幂等键和唯一约束 |
| 主 Agent 形成特殊分支 | 主 Agent 只是默认选择，不是特殊类型 |

## 18. 消融实验

所有实验使用相同输入、模型、数据集和评价指标，每次只移除一个机制。

| 机制 | 完整方案 | 消融方案 | 主要指标 |
| --- | --- | --- | --- |
| Agent 独立记忆 | 按 user_id + agent_id 隔离 | 所有 Agent 共用记忆 | 跨 Agent 泄漏率、记忆相关性 |
| 配置驱动 Agent | 页面创建新 Agent | 每个场景增加代码 | 是否改代码、是否重新部署 |
| Agent 版本 | 会话固定版本 | 每轮读取当前版本 | 会话内提示词指纹变化 |
| 工具调用 | 使用评分工具 | 直接要求模型评分 | 评分一致性、依据完整度 |
| 知识检索 | 绑定知识库并检索 | 不执行检索 | 正确率、无依据回答比例 |
| 流式输出 | SSE 或 WebSocket | 一次性返回 | 首 Token 延迟、最终内容一致性 |
| 异步评价 | worker 异步执行 | 请求内同步执行 | 用户响应延迟、评价完整率 |
| Redis | 保存短时状态和缓存 | 禁用 Redis | 正确性、数据完整性、延迟 |

关键验收结果：

- 跨 Agent 记忆泄漏率为零。
- 新增 Agent 不需要修改运行代码。
- 发布新版本不改变旧会话行为。
- 工具评分比模型自由生成评分更稳定。
- 异步评价不阻塞主对话响应。
- Redis 只影响性能，不影响业务事实。

## 19. 测试和启动

测试覆盖：

- Agent 状态和版本转换。
- 提示词和绑定校验。
- 会话版本绑定。
- 记忆作用域。
- 工具权限。
- PostgreSQL 事务和并发约束。
- SSE、WebSocket 事件顺序和中断。
- worker 任务幂等。
- Agent 创建、发布和会话运行的完整链路。

启动顺序：

~~~text
加载配置
→ 校验配置
→ 初始化 PostgreSQL、Redis 和对象存储
→ 初始化模型、工具和 Agent Runtime
→ 注册路由或任务处理器
→ 启动进程
~~~

关闭顺序：

~~~text
停止接收新请求
→ 停止领取新任务
→ 取消根 context
→ 等待流式请求和当前任务结束
→ 关闭 HTTP、WebSocket、数据库和 Redis
~~~

当前不加入：

~~~text
agent/scenario/
agent/orchestration/
agent/main-agent/
每个 Agent 一套独立运行代码
全局 active_agent 单例
跨模块 common model
没有版本的动态配置
首期独立事件总线
~~~
