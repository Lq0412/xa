# Web 页面结构

两个前端共用 Go API，工程、构建产物、开发端口和登录会话各自独立。用户端面向正式用户，管理端维护平台资源。Flutter 是否接入在 Web 主线完成后按需求决定。

## 管理端 `web/`

采用 React 19、Umi 4、Tailwind CSS 4、Radix UI、Lucide 和 TanStack Table，只有管理员能使用管理页面。普通用户注册入口指向独立用户端。

| 路由 | 模块 | 当前状态 |
| --- | --- | --- |
| `/overview` | 今天 | 确认事项统计与管理入口 |
| `/agents` | Agent 管理 | 真实查询、配置保存、版本检查、启停和删除 |
| `/agents/new`、`/agents/:id` | Agent 配置 | 整页创建和编辑，保留未编辑的配置与绑定 |
| `/approvals` | 确认记录 | 只读查看脱敏记录 |
| `/runs`、`/runs/:id` | 运行追踪 | 当前用户运行列表、详情、spans 与工具结果 |

菜单只保留已接入的管理页面，后续模块统一记录在 ROADMAP 的 Icebox 中。已删除的占位页面地址返回 404。

## 用户端 `web-user/`

使用 React 19、Umi Max 4、Tailwind CSS、shadcn/ui 和 Lucide 图标，不启用 Ant Design。四栏为图标导航、我的 Agent 列表、消息和输入区、可收起的配置栏。窄屏使用 dialog 显示列表与配置，提供焦点约束及 Escape 关闭。

| 路由 | 模块 | 当前状态 |
| --- | --- | --- |
| `/explore` | 发现 Agent | 匿名读取真实公共 Agent，支持搜索和进入对话布局 |
| `/my-agents` | 我的 Agent | 按当前用户读取私有 Agent；真实创建名称、介绍和提示词 |
| `/chat?agentId=<id>&conversationId=<id>` | 对话 | 一个 Agent 一条对话；真实 SSE、Markdown 代码高亮与复制、历史分页、停止生成；首次发送前获取或创建对话 |
| `/login`、`/register` | 账号认证 | 真实注册登录、会话恢复与退出 |

公共列表为空时展示真实空状态，不创建演示 Agent。注册自动创建不可删除的主 Agent，创建其他私有 Agent 后可直接进入对话。普通用户可在配置栏编辑自己的名称、简介、SOUL 和模型；公共 Agent 在用户端只读。管理员继续在管理端维护有权管理的 Agent。

对话界面按 `AgentSidebar`、`MessageList`、`MessageComposer`、`AgentDetails` 拆分。右侧提供“动态、记忆、设置”：动态展示活动与确认事项，记忆面板管理条目与任务，设置提供模型、SOUL、保存或删除记忆前的确认开关及高级工具配置。配置保存使用版本校验，公共 Agent 在用户端只读。

`useChatSession` 管理消息、草稿、运行恢复与审批；`services/chat.ts` 处理 SSE v2，普通 JSON 请求使用生成客户端。`ProcessRail` 展示工具过程，`ApprovalDock` 处理确认，工具结果可查看详情。样式使用 Tailwind，入口是 `tailwind.css`。

会话历史在对话中分页加载，个人记忆位于右侧面板；没有独立 `/history` 或 `/memory` 页面。学习反馈与账号设置没有独立业务页面；当前账号身份和退出登录保留在导航中。

## 启动与部署

管理端 `make web`，用户端 `make web-user`，默认分别访问 `http://localhost:8000` 与 `http://localhost:8002`。两个工程各自输出 `dist/`，部署时分别配置页面路由回退及 `/api/` 反向代理。

管理端通过构建环境变量 `XA_USER_WEB_URL` 配置用户端地址；用户端通过 `XA_ADMIN_WEB_URL` 配置后台地址。生产构建不内置本机地址；未配置时不展示跨端链接。两个入口均使用后端的同一账号体系，分别建立会话，链接不携带令牌。

## 验证

- `make check`：接口生成一致性、Go 格式与 test / vet / build，以及两个 Web 工程的 Biome、TypeScript 和生产构建。
- `cd web-user && npm run test:unit`：用户端既有逻辑单元测试；`npm test` 使用真实后端验证对话、工具、记忆、审批和恢复。
- `cd web && npm test`：使用真实后端、PostgreSQL 和 Redis，覆盖管理端 Agent 操作与版本冲突、登录异常、可用模块导航与刷新、已删除路由的 404、用户端真实创建与独立会话、对话布局、窄屏导航、重复注册及错误密码。
- `XA_BROWSER_INTEGRATION=1 make integration`：生产前端接独立 server，使用真实 `qwen3.8-flash` 验证发送、Markdown、复制代码、停止后保存、历史分页、草稿隔离和窄屏焦点。
- 从 Windows 请求两个 `localhost` 前端及其 `/health/ready`，检查 WSL 访问与后端代理。

日常浏览器测试账号通过环境变量提供，模型浏览器测试自动注册独立账号并清理其数据。验收使用真实后端，界面变更随 PR 附实际截图。知识检索、语音和评价尚未接入。
