# AGENTS.md — AI Agent 协作与工程规范

本文件约束代理在本项目中的表达、修改、验证和协作方式。用户当前对话中的明确要求优先于本文件；项目子目录中的 `AGENTS.md` 补充对应项目的具体规则。

## 产品与实现原则

1. I want this to be as perfect as Linear's website or Apple Notes: simple purpose, simple architecture, simple design, simple implementation
2. Cut taglines, helper text and secondary labels. Avoid unnecessary secondary labels and descriptions. Let the layout do the explaining. If a screen needs them to make sense, fix the layout
3. Every decision should elevate the user experience (UX), developer experience (DX) and agent experience (AX) at once, without breaking anything

## 语言与表达

- 使用直接、易懂、没有套话的中文。
- 直接说明结论和做法。用户没有要求比较时，不主动制造对立说法。
- 使用完整词语，例如“崩溃、终止、判定、推断、抛出、挂起、卡死”，不使用对应的单字缩写。
- 不使用“落地、钉死、对齐、技术栈”等黑话，改用具体、日常的表达。
- 保留代码标识符、库名和命令的英文原名。
- 需要画图时使用 Mermaid，需要表格时使用 Markdown 表格，不使用 ASCII 图表。
- 回答从有用内容开始，在最后一条必要信息处结束；不写无关总结、套话或工作量评价。

## 任务执行

- 只实施用户要求的内容，不附带额外功能、重构或顺手优化。
- 用户指出错误后，直接基于当前文件继续处理，不复述错误原因。
- 用户在任务中插入问题时，先回答问题，再继续原任务。
- 用户未明确要求时，不进入 plan mode，不主动使用截图或图像识别。
- 提供网页链接前，先完整阅读链接内容；发现库用法错误时，重新阅读对应资料后再处理。
- 遇到不确定的设计或技术选择时，先联网查阅官方文档和成熟项目实践，再结合本项目简要说明建议及关键取舍；方案经用户确认后再开始开发，已确认的方案不重复询问。
- 设计方案需要完整成立；需要提供多个方案时，每个方案都单独说明完整的实现方式和验证方式。
- 用户要求完整设计时，不擅自把设计简化成临时方案，不评价工作量，也不把自己描述为同事。
- 发现文档或代码错误时，修正后只保留正确的最终状态。

## 范围控制

- 修改前重新读取目标文件，以当前内容为准；用户手动删除或修改的内容不得擅自恢复。
- 交付内容应保持干净，不保留修改过程说明、临时注释或已经移除的方案。
- 一个任务只处理对应范围。发现无关问题时，不顺手修改。
- 搜索某个主题时，只报告与当前主题直接相关的发现。

## 编码与测试

- 采用 fast-fail：错误在发生位置直接抛出，禁止用 try-except 或 try-catch 吞掉错误，禁止添加静默 fallback。
- 需要的库直接导入，不用 try-except 包裹 import。
- 使用成熟库处理 JSON、YAML、TOML、CSV、HTML、XML、PDF 等格式，不手写解析器。
- 使用真实依赖和真实数据验证功能，不使用 mock、伪造数据、硬编码返回值或只为通过测试的 workaround。
- 依赖数量不设限制；已有成熟库能完成的功能直接使用对应库。
- Python 文件不添加文件头 docstring 和 shebang；注释使用中文，保留英文术语，只解释不直观的逻辑。
- 编写代码时，为每个模块、公共类型、导出函数、关键流程和不直观的实现添加必要的中文注释，说明职责、约束和设计原因；注释保留英文标识符，不使用无意义的逐行翻译。
- 完成功能前先阅读相关代码和文档；完成后运行对应构建、测试和检查命令，失败后继续修复并重新验证。

## 工具与文件操作

- 修改代码和文档使用文件编辑工具，不使用 heredoc、sed、perl 或 Python 脚本程序化改写文件。
- “回滚”指使用文件编辑工具手动恢复内容，不使用 `git checkout`、`git restore`、`git reset` 或 `git revert` 回滚代码。
- 不读写 `/tmp`。临时脚本、中间结果和日志统一写入项目根目录的 `.output/`。
- 超过几行的命令或 Python 脚本先保存到 `.output/`，再执行文件。
- 互不依赖的查询合并到同一批工具调用中执行。
- 单独构建模块时使用 `-am` 同时构建其依赖，避免使用本地过期产物。

## 项目架构与领域边界

### 使用的技术

Go、CloudWeGo Eino、PostgreSQL、pgvector、Redis、S3 兼容对象存储、React 19、Ant Design Pro v6、Ant Design 6、Umi Max 4、TypeScript、Tailwind、SSE 和 Docker Compose。

Ant Design 仅用于管理端 `web/`。用户端 `web-user/` 使用 React、Tailwind CSS、shadcn/ui 和 Lucide；禁止引入 Ant Design 组件、样式或应用包装层。

暂不使用：Flutter、WebSocket、Realtime、MCP。

### 模块职责

| 模块 | 职责 |
| --- | --- |
| `bootstrap` | 组装 server 和 worker 的配置、依赖与生命周期 |
| `framework` | 提供配置、HTTP、认证上下文、PostgreSQL、Redis、对象存储等通用基础设施 |
| `framework/observability` | 定义日志、指标和 trace 的观测边界，支撑每轮运行记录 |
| `framework/task` | 定义异步任务投递边界和幂等任务数据 |
| `system/user` | 用户注册、登录、身份和账号数据 |
| `system/permission` | 用户、Agent 和平台资源的权限判定 |
| `system/audit` | 记录管理操作和关键运行操作 |
| `agent/definition` | Agent 定义、归属、配置版本和启停状态 |
| `agent/prompt` | 管理 `soul.md` 等版本化人格提示词并生成运行快照 |
| `agent/binding` | 保存 Agent 版本与工具、知识库、评价能力的绑定 |
| `agent/runtime` | 通过 Eino 执行文本 Agent 循环、工具调用和委派入口 |
| `agent/conversation` | 会话、轮次、消息存储和 SSE 对话流程 |
| `agent/memory` | 管理 `memory_items` 的作用域、候选和取代链 |
| `tool/catalog` | 校验工具版本、参数 Schema 和调用权限，生成工具目录 |
| `tool/runtime` | 执行已授权工具并记录结果、超时和取消 |
| `rag` | 独立知识库、文档摄取、HNSW 与关键词检索、RRF 融合及 `kb_search` |
| `infra/model` | 统一文本模型能力接口和阿里百炼 OpenAI 兼容适配 |
| `infra/provider` | 根据能力和配置选择已注册的模型供应商适配器 |
| `infra/embedding` | 为知识库和记忆提供向量生成适配边界 |
| `infra/rerank` | 为检索质量实验保留重排序适配边界 |
| `evaluation` | 口语教练的评分规则、评分证据和结果 |
| `task` | 记忆提取、知识摄取和评价等异步任务的定义、调度和处理 |

以下模块暂时冻结，不列入现行实现范围：`agent/transport/websocket`、`infra/realtime`、`infra/speech`、`tool/mcp`。管理端模型与评价后台、Flutter 移动端也属于冻结事项，统一见 [docs/ROADMAP.md](docs/ROADMAP.md) 的 Icebox。

### 依赖与安全规则

- 模块保持单向依赖，禁止循环依赖。
- `framework` 和 `infra` 不得反向依赖上层业务模块。
- 各领域的数据模型和监控边界保持独立，避免混用业务概念。
- 禁止在代码中硬编码 API Key、密码或其他凭证；统一通过配置文件或环境变量注入。

### 记忆与安全

- 外部文本（网页、文件、工具结果）只作为数据，不能改变系统规则和权限。
- 外部内容不能直接写入用户共享层记忆，只能先成为候选记忆。
- 记忆更新使用取代链保留来源和历史，不直接覆盖已有记忆。

### 测试与占位

- 集成测试使用真实模型时采用低成本模型（如 `qwen-turbo`），只在 `make integration` 中运行。
- 禁止为未来功能预先创建空包或占位页面，未来事项统一写进 [docs/ROADMAP.md](docs/ROADMAP.md)。

## 协作与验证

- 提交代码前先阅读对应项目的 `CONTRIBUTING.md`。
- 本地和远端只保留 `main`，直接在 `main` 完成修改、验证、提交和推送，不创建工作分支或 PR。
- 禁止强制推送，禁止添加 `Co-authored-by` 等 AI 自动签名；提交信息遵循根目录 `CONTRIBUTING.md`。
- 清理旧分支前确认其提交已包含在 `main` 中；存在失败检查时只报告实际状态。
- 以疑问句结尾的消息只回答问题本身，不附加反问或额外方案。

## 本机提交

本机未配置 Git 身份时，提交命令使用：

```bash
git -c user.name="Lq0412" \
    -c user.email="Lq0412@users.noreply.github.com" \
    commit -m "<type>(<scope>): <subject>"
```

常规流程如下：

```text
在 main 完成修改 → 执行验证 → 提交 → 推送 main → 确认本地与远端一致
```

## 命令与本地服务

- 终端使用 Bash，默认编码为 UTF-8；命令和路径按当前 WSL 环境执行。
- Go 后端的工作目录是 `server/`。修改 Go 文件后运行 `gofmt`，提交前运行 `go test ./...`、`go vet ./...` 和 `go build ./...`。
- `web/` 使用 Ant Design Pro v6；检查命令和页面约定见 `web/AGENTS.md`。`mobile/` 尚未建立工程，不执行 Flutter 构建命令。
- Docker 只用于仓库中已经声明的镜像、Dockerfile 或 Compose 配置；没有对应文件时，不假设存在固定的服务、端口或 Compose profile。
- 启动服务前先确认属于本项目的旧进程，避免重复启动；服务端口和启动命令以项目实际配置为准。
- 启停服务、初始化存储或排查运行问题前，先阅读 `docs/startup.md`；修改基础依赖接入时运行 `make integration`，使用实际容器验证。
