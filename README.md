# xa

Agent 管理与运行平台，后端使用 Go。通过配置创建公共或私有 Agent，支持英语陪练及其他学习任务。

当前已建立 server、worker 运行基础，完成账号注册登录、Redis 会话与公共及私有 Agent 管理接口。两个独立 Web 工程提供管理端模块导航、用户端 Agent 列表和对话布局；尚未实现的模块明确标注待开发。文本模型、Realtime 和邮件验证仍待接入，移动端最后开发。

目录说明：

- `server/`：Go 后端服务和异步任务进程
- `web/`：独立 Web 管理端，使用 Umi 4、React、Tailwind CSS、Radix UI 和 TanStack Table
- `web-user/`：独立 Web 用户端，提供 Agent 发现、创建和对话入口
- `mobile/`：英语陪练移动端
- `docs/`：架构和产品设计文档
- `reference/`：仅用于参考的独立项目

接口文档的查看与维护方式见 [接口文档说明](docs/api/README.md)。

本地启动：

```bash
make up
```

`make up` 先释放 18080、18081、8000、8002，再启动 PostgreSQL、Redis、对象存储并执行迁移，然后在后台运行 server、worker、管理端和用户端。日志写入 `.output/logs/`，进程号写入 `.output/pids/`。就绪后访问管理端 http://localhost:8000 ，用户端 http://localhost:8002 。

`make status` 查看各进程和端口。`make down` 停止这些进程，保留 Docker 依赖；`make down ALL=1` 同时停止依赖。首次使用先执行 `make env`。两个前端若还没有依赖，先分别执行 `make web-install` 和 `make web-user-install`。

也可以分终端手动启动：

```bash
make env
make deps-up
make server
```

另开终端运行 `make worker`。管理端先执行 `make web-install`，再执行 `make web`，访问 `http://localhost:8000`。用户端先执行 `make web-user-install`，再在独立终端执行 `make web-user`，访问 `http://localhost:8002`。配置、端口和验证命令见 [启动说明](docs/startup.md)，页面范围见 [前端结构](docs/frontend-structure.md)，后续安排见 [开发计划](docs/development-plan.md)。
