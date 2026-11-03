# 本地启动

当前可运行内容：server、worker 生命周期，PostgreSQL、Redis、S3 真实连接和健康检查。Agent、账号、任务处理器仍为接口骨架，Web 和移动端尚未建立工程。

## 1. 环境

- WSL / Linux、Bash、Go 1.26 或更新版本、Make、OpenSSL；race 检查需要 GCC 等 C 编译器。
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

配置无效或任一依赖无法连接时，进程直接退出且返回非零状态。worker 当前只运行依赖检查和生命周期，尚不领取业务任务。

```bash
curl --fail http://127.0.0.1:18080/health/live
curl --fail http://127.0.0.1:18080/health/ready
curl --fail http://127.0.0.1:18081/health/ready
```

`live` 表示 HTTP 进程存活；`ready` 实际检查数据库、Redis 和桶访问，任一失败返回 503。它不代表尚未接入的模型或业务接口可用。

## 4. 验证和停止

```bash
make check
make integration
```

集成测试需要已启动的真实依赖，验证事务、缓存、对象上传读取删除、依赖异常、HTTP 启停和端口冲突。使用实际项目文件作为存储内容；测试创建的对象和缓存键会清理，不删除业务数据。

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
