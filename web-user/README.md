# xa Web 用户端

React 19、Umi Max 4、Tailwind CSS、shadcn/ui 和 Lucide 图标；用户端不使用 Ant Design。后端提供注册、Agent、版本配置与 SSE 对话接口。

## 运行与验证

根目录先运行 `make migrate`、`make server`，再运行 `make web-user`，访问 `http://localhost:8002`。默认 API 代理为 `127.0.0.1:18080`，可用 `XA_API_PROXY` 覆盖。构建产物为本目录 `dist/`。

本目录执行 `npm run lint`、`npm run build`。真实浏览器验收运行 `XA_BROWSER_INTEGRATION=1 make integration`，自动构建用户端，启动独立 server，注册独立账号，使用真实 PostgreSQL 与低成本 `qwen3.8-flash`，结束后清理本次数据。需要 Playwright Chromium；临时文件在根目录 `.output/`，不记录截图或视频。

根目录的 `docs/api/openapi.yaml` 更新后，在本目录执行 `npm run openapi`，并提交 `src/services/generated/` 的生成结果。

## 页面边界

左侧图标栏只有对话、发现 Agent、我的 Agent；设置、账号、退出在底部。第二栏按名称搜索，分成「我的 Agent」和「已添加」，主 Agent 根据后端 `is_primary` 置顶，每项显示最近一条消息，不显示会话树。

每个用户与每个 Agent 有一条连续对话。首次发送前调用 `POST /conversations` 获取或创建；URL 为 `/chat?agentId=...&conversationId=...`，只传 Agent 时自动恢复已有对话。向上滚动按消息 ID 游标加载历史；流式过程中阅读历史保持位置，主动停止后保留已经生成的内容。输入支持中文输入法、Enter 发送和 Shift + Enter 换行。

右侧配置栏默认显示真实资料、SOUL 和模型。SOUL 使用 Markdown 阅读视图，点击编辑后提供源文及预览；SOUL、模型分别保存不可变版本，携带乐观锁版本号。公共 Agent 只读；记忆、知识库和工具模块本次不显示。窄屏默认收起，打开时使用 dialog 提供焦点约束和 Escape 关闭；展开偏好存于 localStorage。宽度低于 900px 时 Agent 列表也改为 dialog。

`AgentSidebar`、`MessageList`、`MessageComposer`、`AgentDetails` 负责界面，`useChatSession` 负责真实请求与每个 Agent 的草稿。Markdown 使用 `react-markdown`、`remark-gfm`、`rehype-highlight`；SSE 使用 `eventsource-parser`。设置仅包含已接入的配置栏偏好，管理员另有后台入口。

Ant Design 仅由管理端使用；Umi Max 的开发工具可能含有其传递依赖，但用户端禁用对应插件，源码和生产页面不加载 Ant Design。
