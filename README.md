# xa

Agent 管理与运行平台，后端使用 Go。通过配置创建公共或私有 Agent，支持英语陪练及其他学习任务。

当前已完成账号与 Agent 管理、阿里百炼文本对话、NativeEngine 工具循环、SSE 恢复与取消、工具审批、长期记忆及运行追踪。用户端可管理记忆和工具配置，管理端提供 Agent 配置、确认记录和运行详情。下一阶段为知识库 RAG，随后接入文本版口语教练、FSRS 复习和主 Agent 委派。

目录说明：

- `server/`：Go 后端服务和异步任务进程
- `web/`：独立 Web 管理端，使用 Umi 4、React、Tailwind CSS、Radix UI 和 TanStack Table
- `web-user/`：独立 Web 用户端，提供 Agent 发现、创建和对话入口
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
make migrate
make server
```

另开终端运行 `make worker`。管理端先执行 `make web-install`，再执行 `make web`，访问 `http://localhost:8000`。用户端先执行 `make web-user-install`，再在独立终端执行 `make web-user`，访问 `http://localhost:8002`。配置、端口和验证命令见 [启动说明](docs/startup.md)，页面范围见 [前端结构](docs/frontend-structure.md)，当前状态和后续安排见 [路线图](docs/ROADMAP.md)。
