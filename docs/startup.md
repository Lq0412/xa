# 本地启动

当前可运行内容：server、worker，PostgreSQL、Redis、S3 连接和健康检查；账号与 Agent 管理、百炼文本对话、NativeEngine 工具循环、审批与恢复、记忆 v2、运行追踪，以及两个独立 Web 前端。

## 1. 环境

- WSL / Linux、Bash、Go 1.26 或更新版本、Make、OpenSSL；race 检查需要 GCC 等 C 编译器。
- Web 使用 Node.js 24 和 npm，在仓库根目录执行 `nvm use`。管理端使用普通 Umi 4，用户端使用 Umi Max 4；两端使用 React 19、Tailwind CSS、Radix UI 和 Lucide 图标。
- Docker Engine 与 Compose。WSL 使用 `/usr/bin/docker` 原生客户端；其他安装路径通过 `make DOCKER=/path/to/docker ...` 指定。
- Go module 位于 `server/`；下列 Make 命令均在仓库根目录运行。

## 2. 配置和依赖

```bash
make env
make deps-up
make deps-status
```

`make env` 根据 `.env.example` 创建权限为 600 的 `.env` 并生成随机本地凭证；已有文件不会覆盖。不要提交 `.env`。运行脚本会将其中的变量导入进程，直接运行二进制时需自行注入环境变量。

用户端文本对话需要配置 `XA_BAILIAN_API_KEY` 和 `XA_BAILIAN_BASE_URL`；模型名默认分别为 `XA_CHAT_MODEL=qwen3.7-plus` 与 `XA_LIGHT_MODEL=qwen3.8-flash`。`XA_CHAT_MODELS` 配置 Agent 可选模型白名单，默认 `qwen3.7-plus,qwen3.8-flash`；每轮使用 Agent 当前版本的模型。缺少百炼配置时 server 和 worker 会在启动校验阶段直接退出。密钥只放在本地 `.env`，不会写入日志或版本库。

`make deps-up` 等待三个依赖 healthy，再初始化 S3 存储桶。新 PostgreSQL 数据卷自动启用 pgvector；不会创建业务表。启动后端前必须确认依赖健康且桶初始化成功。

| 服务 | 默认本机地址 |
| --- | --- |
| server | `127.0.0.1:18080` |
| Web 管理端 | `localhost:8000` |
| Web 用户端 | `localhost:8002` |
| worker 健康接口 | `127.0.0.1:18081` |
| PostgreSQL | `127.0.0.1:15432` |
| Redis | `127.0.0.1:16379` |
| S3 API | `127.0.0.1:19000` |
| 对象存储控制台 | `127.0.0.1:19001` |

容器使用 `xa` 项目名和独立数据卷，端口只绑定本机。修改 Redis 或 S3 端口时，同时修改 `.env` 中的地址和对应 `*_PORT`。对象存储使用固定镜像摘要的 RustFS `1.0.0-alpha.72`，该预发布版本仅用于本地 S3 协议验证。生产环境另行选择稳定 S3 服务、独立凭证、TLS 和受控存储权限。

存储桶由一次性命令 `cmd/init-storage` 显式创建，重复执行只检查已存在的桶。server 和 worker 启动时只检查依赖，不自动创建存储资源。

## 3. 启动进程

先执行一次数据库迁移，再用 `ss -ltnp` 检查项目配置的端口，确认没有旧的本项目进程；不要终止其他项目的服务：

```bash
make migrate
```

server 和 worker 启动时只检查迁移版本；版本不是最新时会直接退出。在两个终端分别执行：

```bash
make server
```

```bash
make worker
```

Web 管理端在第三个终端启动：

```bash
make web-install
make web
```

管理端访问 `http://localhost:8000`，使用管理员账号登录。支持今天、Agent 列表与整页配置、只读确认记录、运行列表与详情；未实现模块不列入菜单。

Web 用户端在独立终端启动：

```bash
make web-user-install
make web-user
```

用户端访问 `http://localhost:8002`，支持匿名浏览公共 Agent、注册登录、创建私有 Agent 和流式文本对话。注册自动创建主 Agent；每个 Agent 保留一条连续对话，点击左侧 Agent 即可继续，向上滚动加载更早消息。右侧可编辑自己的 SOUL 和模型，保存生成新版本；公共 Agent 只读。停止生成后保留已经生成的内容。Windows 访问 WSL 优先使用 `localhost`，本机曾出现 IPv4 `127.0.0.1` 转发失败、`localhost` 正常的情况。

两个前端的 Umi 开发服务器均将 `/api/` 和 `/health/` 请求转发到 `127.0.0.1:18080`，后端地址通过 `XA_API_PROXY` 配置。管理端默认提供到 `http://localhost:8002` 的用户端入口，用户端为管理员提供到 `http://localhost:8000` 的后台入口；可分别通过 `XA_USER_WEB_URL` 和 `XA_ADMIN_WEB_URL` 覆盖。两个入口共用账号，各自登录，不通过跳转 URL 传递令牌。

管理端使用 Umi 4、Tailwind CSS、Radix UI 和 TanStack Table。刷新页面会向后端恢复会话，编辑使用版本校验防止覆盖并发修改。用户端右侧提供动态、记忆和设置，支持工具确认及高级配置。真实浏览器验收方法见 [Web 说明](../web/README.md)。

配置无效或任一依赖无法连接时，进程直接退出且返回非零状态。worker 领取 PostgreSQL 中的 `memory_jobs`，执行记忆抽取、滚动摘要、摘要重写和维护；需同时启动 server 和 worker。抽取使用 qwen3.8-flash（关闭思考、严格 JSON Schema），向量使用 qwen3.7-text-embedding（1024 维）。任务失败前两次分别等待 30 秒、2 分钟重试，第三次进入 `dead`，可在用户端重试；失败不改变已完成回复。

```bash
curl --fail http://127.0.0.1:18080/health/live
curl --fail http://127.0.0.1:18080/health/ready
curl --fail http://127.0.0.1:18081/health/ready
```

`live` 表示 HTTP 进程存活；`ready` 实际检查数据库、Redis 和桶访问，任一失败返回 503。它不验证模型调用或完整业务流程。

独立的 `migrate` 命令执行所有待应用迁移，包括账号、Agent、会话、记忆与运行记录。若配置 `XA_ADMIN_EMAIL` 与 `XA_ADMIN_PASSWORD`，server 首次启动会初始化管理员；两项必须同时配置，密码不会写入日志。

完整接口由 Go 生成，见 [接口文档说明](api/README.md)。发送消息与运行事件订阅使用 SSE，普通 JSON 接口覆盖账号、Agent、会话、记忆、工具、审批与运行查询。

`POST /api/v1/conversations` 按 `agent_id` 获取或创建当前用户的唯一对话；消息查询接受 `limit` 和上一页返回的 `before` 游标。

登录返回的令牌通过 `Authorization: Bearer <token>` 传递。普通用户只能创建和修改自己的私有 Agent；公共 Agent 只能由管理员维护。

## 4. 验证和停止

```bash
make check
make integration
make web-build
```

集成测试需要已启动的真实依赖，验证事务、缓存、对象上传读取删除、依赖异常、HTTP 启停和端口冲突。使用实际项目文件作为存储内容；测试创建的对象和缓存键会清理，不删除业务数据。

S1 记忆验收在同一个 PostgreSQL 实例中创建独立临时数据库，执行全部迁移后调用真实百炼，结束时删除该数据库；测试数据库账号需要 CREATEDB 权限，本仓库 Compose 的初始化账号具备该权限。独立数据库避免已运行的 worker 领取测试任务。验收覆盖超过 20 条真实历史后的召回、新 Agent 共享 profile、自动与手动取代链、权限隔离、预算、开关、模型连接失败及失效 Agent 的任务处理。

`make check` 检查接口生成一致性、Go 格式及 test / vet / build，并执行两个 Web 工程的 Biome、TypeScript 和构建。管理端浏览器验收运行 `cd web && npm test`，需要管理员测试账号通过 `XA_E2E_EMAIL`、`XA_E2E_PASSWORD` 注入，并保持两个 Web 和 server 启动。可用 `XA_WEB_URL`、`XA_USER_WEB_URL` 覆盖测试访问地址。用户端既有逻辑测试运行 `cd web-user && npm run test:unit`，浏览器场景运行 `npm test`。

完整对话验收运行 `make integration INTEGRATION_WEB=1`（等同于 `XA_BROWSER_INTEGRATION=1 make integration`），需要已安装 Playwright Chromium 及其系统依赖。它构建用户端并启动独立的真实 server，使用低成本 `qwen3.8-flash` 验证 SOUL 版本、生成、Markdown 与代码复制、停止保存、唯一对话、历史分页、刷新恢复和窄屏焦点；测试账号与数据自动清理，不更改日常服务的模型配置。普通集成测试也使用真实模型验证 SOUL 生效。

前台进程使用 `Ctrl+C`，或向对应进程发送 `SIGTERM`。进程停止接收请求，取消正在运行的健康检查，等待 HTTP 退出，再释放外部连接。关闭超时会返回错误。

```bash
make deps-down
```

此命令保留数据卷。修改 `.env` 中数据库密码不会修改已有卷内的数据库密码，应通过数据库管理命令同步修改。

日志输出到各进程标准输出，排查容器使用：

```bash
/usr/bin/docker compose --env-file .env -f resources/docker/compose.yaml logs --tail=100
```

构建产物和临时日志放在根目录 `.output/`。不要打印解析后的完整配置、连接串或 `.env` 内容。
