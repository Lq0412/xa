# xa Web 用户端

独立 React 19、Ant Design 6、Umi Max 4 工程。通过 Go API 提供公共 Agent 浏览、登录注册、私有 Agent 列表与创建，以及 SSE 流式文本对话。

## 本地运行

先按 [启动说明](../docs/startup.md) 启动后端，再在根目录执行 `make web-user-install` 和 `make web-user`。本次需先运行 `make migrate`，应用公共 Agent 添加记录迁移。访问 `http://localhost:8002`。默认代理地址为 `127.0.0.1:18080`，通过 `XA_API_PROXY` 覆盖。

## 验证与构建

在本目录执行 `npm run lint`、`npm run lint:antd`、`npm run build`。用户端的真实浏览器用例在 `tests/`。日常检查不执行 `max setup`，避免清空运行中的生成目录。

构建产物为本目录 `dist/`。静态服务器需为页面路由回退至 `index.html`，将 `/api/` 代理至 Go server。管理员的后台链接使用构建环境变量 `XA_ADMIN_WEB_URL`；仅开发环境默认 `http://localhost:8000`。

对话页从左到右为图标栏、Agent 列表、对话区和详情栏。图标导航上下居中，设置、账号和退出固定在底部；发现与我的 Agent 页面不重复展示文字导航。窄屏用抽屉显示 Agent 列表和详情，详情默认收起，用户的展开选择保存在 localStorage。

`AgentSidebar` 按名称搜索可用 Agent，并分别展开历史会话。列表使用 `GET /agent-library`，公共 Agent 通过 `PUT /agent-library/{id}` 添加并保存在账号下；已有公共对话会在迁移时纳入列表。主 Agent 仅根据真实 `is_default` 字段置顶和标识，前端不推断主 Agent，也不自动创建主 Agent。

`AgentDetails` 展示真实头像、名称、介绍、可见性、创建时间、系统提示词和 `runtime_model`；缺失字段不展示。公共 Agent 只读，自有私有 Agent 使用共享的 `AgentEditor` 编辑，调用既有配置保存接口，携带版本号并保留未编辑字段。

`useChatSession` 管理会话状态和草稿；`services/chat.ts` 处理聊天 API 与 SSE，`services/http.ts` 统一认证和 HTTP 错误。Markdown 使用 `react-markdown` 与 `remark-gfm`，SSE 使用 `eventsource-parser`。

首次发送自动创建会话，URL 使用 `/chat?agentId=...&conversationId=...`，刷新恢复同一 Agent 和会话；旧的 `agent`、`conversation` 参数自动转换。生成中切换会话仍更新原会话，当前页面内的草稿分别保留。停止生成后，已生成的部分会保存到历史。支持 Enter 发送、Shift + Enter 换行和中文输入法；向上阅读历史时暂停自动滚动。

`XA_BROWSER_INTEGRATION=1 make integration` 会构建用户端，启动使用 `qwen3.8-flash` 的独立真实 server，通过 Playwright 验证四栏位置、公共 Agent 添加、创建与编辑、版本配置保留、流式消息、停止保存、历史恢复、窄屏布局和断网错误。需要在本目录安装 Playwright Chromium 及其系统依赖，并配置 `.env` 中的管理员和百炼凭证；临时目录通过 `TMPDIR` 指向根目录 `.output/`。测试创建独立账号和 Agent，结束后清理自己的数据。普通 Go 检查不启动浏览器，直接 `npm test` 不允许调用真实模型。

设置入口只提供已经接入的详情显示选项；个人记忆、学习反馈和账号资料编辑尚未接入，安排见 ROADMAP 的 Icebox。
