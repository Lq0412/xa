# xa

Agent 管理与运行平台，后端使用 Go。通过配置创建公共或私有 Agent，支持英语陪练及其他学习任务。

当前已建立 server、worker 运行基础，并完成账号注册登录、Redis 会话、公共与私有 Agent 管理接口。Web 和移动端暂为空目录；文本模型、Realtime 和邮件验证仍待接入。

目录说明：

- `server/`：Go 后端服务和异步任务进程
- `web/`：Agent 管理 Web 端
- `mobile/`：英语陪练移动端
- `docs/`：架构和产品设计文档
- `reference/`：仅用于参考的独立项目

本地启动：

```bash
make env
make deps-up
make server
```

另开终端运行 `make worker`。配置、端口和验证命令见 [启动说明](docs/startup.md)，后续安排见 [开发计划](docs/development-plan.md)。
