# 本地启动

当前可运行内容：server、worker 生命周期，PostgreSQL、Redis、S3 真实连接和健康检查；账号注册登录、Redis 会话、公共与私有 Agent 管理接口，以及两个独立 Web 前端。管理端提供平台概览和 Agent 管理，用户端提供真实 Agent 列表、创建与对话布局。文本模型、Realtime、邮件验证、异步任务处理器和移动端仍未接入。

## 1. 环境

- WSL / Linux、Bash、Go 1.26 或更新版本、Make、OpenSSL；race 检查需要 GCC 等 C 编译器。
- Web 使用 Node.js 22 或更新版本和 npm；框架为 React 19、Ant Design 6 与 Umi Max 4。
- Docker Engine 与 Compose。WSL 使用 `/usr/bin/docker` 原生客户端；其他安装路径通过 `make DOCKER=/path/to/docker ...` 指定。
- Go module 位于 `server/`；下列 Make 命令均在仓库根目录运行。

## 2. 配置和依赖

```bash
make env
make deps-up
make deps-status
```

`make env` 根据 `.env.example` 创建权限为 600 的 `.env` 并生成随机本地凭证；已有文件不会覆盖。不要提交 `.env`。运行脚本会将其中的变量导入进程，直接运行二进制时需自行注入环境变量。

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

先用 `ss -ltnp` 检查项目配置的端口，确认没有旧的本项目进程；不要终止其他项目的服务。在两个终端分别执行：

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

管理端访问 `http://localhost:8000`，使用管理员账号登录。现有 Agent 管理支持筛选、查看、创建、编辑、启停和删除；平台概览提供管理入口，未实现模块不列入菜单。

Web 用户端在独立终端启动：

```bash
make web-user-install
make web-user
```

用户端访问 `http://localhost:8002`，支持匿名浏览公共 Agent、注册登录、查看和创建自己的私有 Agent，并进入对话布局。对话页尚未接入模型调用，发送操作禁用；会话历史、个人记忆、学习反馈和账号设置的占位页面与导航已移除。Windows 访问 WSL 优先使用 `localhost`，本机曾出现 IPv4 `127.0.0.1` 转发失败、`localhost` 正常的情况。

两个前端的 Umi 开发服务器均将 `/api/` 和 `/health/` 请求转发到 `127.0.0.1:18080`，后端地址通过 `XA_API_PROXY` 配置。管理端默认提供到 `http://localhost:8002` 的用户端入口，用户端为管理员提供到 `http://localhost:8000` 的后台入口；可分别通过 `XA_USER_WEB_URL` 和 `XA_ADMIN_WEB_URL` 覆盖。两个入口共用账号，各自登录，不通过跳转 URL 传递令牌。

Web 使用 ProLayout 侧边导航、ProTable 列表和 ModalForm 编辑窗，框架来源为 Ant Design Pro v6.0.3。刷新页面会向后端恢复会话，编辑使用版本校验防止覆盖并发修改。真实浏览器验收方法见 [Web 说明](../web/README.md)。

配置无效或任一依赖无法连接时，进程直接退出且返回非零状态。worker 当前只运行依赖检查和生命周期，尚不领取业务任务。

```bash
curl --fail http://127.0.0.1:18080/health/live
curl --fail http://127.0.0.1:18080/health/ready
curl --fail http://127.0.0.1:18081/health/ready
```

`live` 表示 HTTP 进程存活；`ready` 实际检查数据库、Redis 和桶访问，任一失败返回 503。它不代表尚未接入的模型或业务接口可用。

server 启动时会执行幂等的身份和 Agent 表迁移。若配置 `XA_ADMIN_EMAIL` 与 `XA_ADMIN_PASSWORD`，首次启动会初始化管理员；两项必须同时配置，密码不会写入日志。

已接入的接口包括：

- `POST /api/v1/auth/register`、`POST /api/v1/auth/login`、`POST /api/v1/auth/logout`、`GET /api/v1/auth/me`
- `GET /api/v1/available-agents`
- `GET/POST /api/v1/agents`、`GET /api/v1/agents/{agent_id}`、`GET/PUT /api/v1/agents/{agent_id}/config`
- `POST /api/v1/agents/{agent_id}/enable`、`POST /api/v1/agents/{agent_id}/offline`、`DELETE /api/v1/agents/{agent_id}`

登录返回的令牌通过 `Authorization: Bearer <token>` 传递。普通用户只能创建和修改自己的私有 Agent；公共 Agent 只能由管理员维护。

## 4. 验证和停止

```bash
make check
make integration
make web-build
```

集成测试需要已启动的真实依赖，验证事务、缓存、对象上传读取删除、依赖异常、HTTP 启停和端口冲突。使用实际项目文件作为存储内容；测试创建的对象和缓存键会清理，不删除业务数据。

`make check` 同时执行两个 Web 工程的 Biome、TypeScript、antd 检查和构建。浏览器验收统一运行 `cd web && npm test`，需要管理员测试账号通过 `XA_E2E_EMAIL`、`XA_E2E_PASSWORD` 注入，并保持两个 Web 和 server 启动。可用 `XA_WEB_URL`、`XA_USER_WEB_URL` 覆盖测试访问地址。

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
