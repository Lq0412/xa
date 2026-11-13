# xa Web 用户端

React 19、Umi Max 4、Tailwind CSS、shadcn/ui 和 Lucide 图标；用户端不使用 Ant Design。后端提供注册、Agent、版本配置与 SSE 对话接口。

## 运行与验证

使用 Node.js 24，在仓库根目录执行 `nvm use`。

根目录先运行 `make migrate`、`make server`，再运行 `make web-user`，访问 `http://localhost:8002`。默认 API 代理为 `127.0.0.1:18080`，可用 `XA_API_PROXY` 覆盖。构建产物为本目录 `dist/`。

本目录执行 `npm run lint`、`npm run build`。真实浏览器验收运行 `XA_BROWSER_INTEGRATION=1 make integration`，自动构建用户端，启动独立 server，注册独立账号，使用真实 PostgreSQL 与低成本 `qwen3.8-flash`，结束后清理本次数据。需要 Playwright Chromium；临时文件在根目录 `.output/`，不记录截图或视频。

接口变更先修改 Go 输入输出和 Huma operation，再在根目录运行 `make api`、`make api-client`，提交生成的 OpenAPI 与客户端。禁止手改生成文件，规则见 [接口约定](../docs/api/README.md)。逻辑测试使用 `npm run test:unit`，现有浏览器场景使用 `npm test` 连接真实后端。

## 页面边界

左侧图标栏只有对话、发现 Agent、我的 Agent；设置、账号、退出在底部。第二栏按名称搜索，分成「我的 Agent」和「已添加」，主 Agent 根据后端 `is_primary` 置顶，每项显示最近一条消息，不显示会话树。

每个用户与每个 Agent 有一条连续对话。首次发送前调用 `POST /conversations` 获取或创建；URL 为 `/chat?agentId=...&conversationId=...`，只传 Agent 时自动恢复已有对话。向上滚动按消息 ID 游标加载历史；流式过程中阅读历史保持位置，主动停止后保留已经生成的内容。输入支持中文输入法、Enter 发送和 Shift + Enter 换行。

右侧栏分为“动态、记忆、设置”。动态显示活动和待确认操作；记忆面板提供条目查看、编辑、归档恢复及失败任务重试；设置显示简介、模型、SOUL、保存或删除记忆前的确认开关，高级入口提供工具与运行限制配置。保存产生不可变版本并携带乐观锁版本号，公共 Agent 配置只读。知识库尚未接入。窄屏打开侧栏使用 dialog 提供焦点约束和 Escape 关闭，展开偏好存于 localStorage。

`AgentSidebar`、`MessageList`、`MessageComposer`、`AgentDetails` 负责界面，`useChatSession` 负责真实请求与每个 Agent 的草稿。Markdown 使用 `react-markdown`、`remark-gfm`、`rehype-highlight`；SSE 使用 `eventsource-parser`。设置仅包含已接入的配置栏偏好，管理员另有后台入口。

用户端使用 SSE v2 展示文本、工具过程、审批和终态，刷新与断线后按服务端活动运行恢复；停止按钮调用取消接口。`ProcessRail`、`ApprovalDock` 和 `ToolResultDetails` 分别展示过程、确认和结果详情。

两端均不使用 Ant Design 组件；Umi Max 开发工具可能含有其传递依赖，用户端禁用对应插件，源码和生产页面不加载 Ant Design。
