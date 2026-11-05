# Web 页面结构

两个前端共用 Go API，工程、构建产物、开发端口和登录会话各自独立。用户端面向正式用户，管理端维护平台资源。Flutter 在 Web 能力完整验证后开发。

## 管理端 `web/`

采用 Ant Design Pro v6，只有管理员能使用管理页面。普通用户注册入口指向独立用户端。

| 路由 | 模块 | 当前状态 |
| --- | --- | --- |
| `/overview` | 平台概览 | 提供 Agent 管理入口与功能说明 |
| `/agents` | Agent 管理 | 真实查询、配置保存、版本检查、启停和删除 |

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

对话界面按 `AgentSidebar`、`MessageList`、`MessageComposer`、`AgentDetails` 拆分，页面只负责组合。SOUL 默认显示 Markdown，点击编辑后可切换源文与预览；SOUL 和模型分别保存新版本，公共 Agent 只读。`useChatSession` 按会话 ID 保存消息、草稿和请求状态，URL 决定当前选择。`services/chat.ts` 处理聊天接口和 SSE 解析，`services/http.ts` 统一认证和错误；样式使用 Tailwind，入口是 `tailwind.css`。

会话历史、个人记忆、学习反馈和账号设置的独立页面与入口已移除，旧地址返回 404；当前账号身份和退出登录保留在导航中。

## 启动与部署

管理端 `make web`，用户端 `make web-user`，默认分别访问 `http://localhost:8000` 与 `http://localhost:8002`。两个工程各自输出 `dist/`，部署时分别配置页面路由回退及 `/api/` 反向代理。

管理端通过构建环境变量 `XA_USER_WEB_URL` 配置用户端地址；用户端通过 `XA_ADMIN_WEB_URL` 配置后台地址。生产构建不内置本机地址；未配置时不展示跨端链接。两个入口均使用后端的同一账号体系，分别建立会话，链接不携带令牌。

## 验证

- `make check`：Go test、vet、build，以及两个 Web 工程的 Biome、TypeScript 和生产构建；Ant Design 检查只运行于管理端。
- `cd web && npm test`：使用真实后端、PostgreSQL 和 Redis，覆盖管理端 Agent 操作与版本冲突、登录异常、可用模块导航与刷新、已删除路由的 404、用户端真实创建与独立会话、对话布局、窄屏导航、重复注册及错误密码。
- `XA_BROWSER_INTEGRATION=1 make integration`：生产前端接独立 server，使用真实 `qwen3.8-flash` 验证发送、Markdown、复制代码、停止后保存、历史分页、草稿隔离和窄屏焦点。
- 从 Windows 请求两个 `localhost` 前端及其 `/health/ready`，检查 WSL 访问与后端代理。

日常浏览器测试账号通过环境变量提供，模型浏览器测试自动注册独立账号并清理其数据。测试不替换接口响应，不采集截图或录屏。知识检索、语音、记忆和评价尚未接入此页面。
