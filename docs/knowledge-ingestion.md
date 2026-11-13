# 知识库 R1：文档摄取

R1 提供知识库、文档和切片接口，并在现有 worker 内完成解析、切片、中文分词、向量化和异步删除。接口以 Go / Huma 生成的 [OpenAPI](api/openapi.yaml) 为准。

## 权限与接口

知识库有唯一所有者。私有库仅所有者可读写，管理员访问他人私有库也返回 404；列表不包含他人私有库。公共库仅管理员可创建和维护，登录用户可以读取。同一 workspace 内列出自己的库与公共库。

| 方法 | `/api/v1` 下的路径 | 结果 |
| --- | --- | --- |
| GET / POST | `/knowledge-bases` | 分页列出 / 创建，默认 private |
| GET / PATCH / DELETE | `/knowledge-bases/{id}` | 详情 / 更新名称或描述 / 软删除并排队清理 |
| GET / POST | `/knowledge-bases/{id}/documents` | 文档状态列表 / multipart 的 `file` 字段上传 |
| GET / DELETE | `/knowledge-bases/{id}/documents/{docId}` | 文档详情 / 软删除并排队清理 |
| POST | `/knowledge-bases/{id}/documents/{docId}/retry` | 仅 failed 文档可重试 |
| GET | `/knowledge-bases/{id}/documents/{docId}/chunks` | 分页查看 ready 文档的切片 |

分页使用 `limit`（默认 20，范围 1–100）与 `offset`（默认 0）。返回 `items`、`total`。创建返回 201；同一知识库相同 SHA-256 原文重复上传返回已有文档和 200，不创建第二条任务。删除返回 204，随后资源和切片访问返回 404。错误体保持 `{code,message,request_id}`。

## 摄取与恢复

1. 上传校验扩展名、内容嗅探、UTF-8 和大小，将原文写入 S3，再在同一事务插入文档与 ingest 任务。配额检查锁定所有者与知识库；公共库原文计入知识库所有者的配额。失败后删除未入库对象。
2. worker 使用 `FOR UPDATE SKIP LOCKED` 锁住任务，文档单独提交为 processing。进程退出后任务事务回滚，下次从头执行。
3. TXT 规范化换行；Markdown 使用 Goldmark 识别标题层级；PDF 用 `pdftotext -layout -enc UTF-8`，依据页分隔符保留页码。平均每页不足 50 个非空白字符时失败，原因是“扫描件，暂不支持 OCR”。
4. 在标题或页范围内按段落、句子、字符边界切片。默认目标 500 token、重叠约 15%、最小约 50 token。估算按中文约 1.5 字/token、ASCII 约 4 字/token；不足 50 token 的尾段并入前块。独立短标题章节或短页保留自己的出处，可短于最小目标。
5. gse 0.80.3 使用内置中文词典预分词，空格连接后写入 `search_text`。PostgreSQL 用 `simple` 生成 `fts` 和 GIN 索引。向量批次不超过 20，使用 `qwen3.7-text-embedding`、1024 维、`text_type=document`，建立 cosine HNSW 索引。
6. 在同一事务内删除文档全部旧切片、写入新切片、更新文档为 ready 和知识库计数、完成任务。失败的向量请求不会留下部分新切片。

`char_start` / `char_end` 是规范化全文的 Unicode 字符位置，左闭右开；PDF 页分隔符占一个字符。Markdown 保存 `heading_path`；PDF 保存 `page_from` / `page_to`。`token_count` 是估算值。每份文档和每个切片记录实际向量模型；`chunker_version` 记录算法、估算器、gse 版本和切片参数。

失败时文档为 failed，并保存用户可读的 `error_code`、`error_message`。常见原因包括编码错误、超页数/字数、扫描件、PDF 损坏、原文读取失败和向量化失败。自动重试沿用记忆队列的 30 秒、2 分钟退避，第三次失败将任务标记为 dead。手动 retry 结束原活动摄取任务，插入新的 ingest 任务并清除文档错误。

删除立即隐藏文档，后台 delete 任务先删除 S3 原文，再物理删除文档与切片；知识库保留软删除行，任务保留完成记录。deleting 原文在物理删除前仍计入总存储配额。每小时清理超过 24 小时且没有文档行引用的原文，覆盖进程在上传后、事务提交前退出的情况。对象前缀包含数据库名，独立验收库不会清理日常数据库的对象。

## 配置与依赖

以下变量通过运行环境注入。默认值集中在 `KnowledgeConfig`，不用修改业务代码。

| 变量 | 默认值 | 含义 |
| --- | --- | --- |
| `XA_KB_MAX_FILE_BYTES` | 20971520 | 单文件 20 MiB |
| `XA_KB_MAX_PDF_PAGES` | 300 | PDF 页数上限 |
| `XA_KB_MAX_CHARACTERS` | 500000 | 解析后的字符上限 |
| `XA_KB_MAX_DOCUMENTS` | 100 | 每库文档数上限 |
| `XA_KB_MAX_BASES_PER_USER` | 10 | 每用户知识库数量上限 |
| `XA_KB_MAX_USER_BYTES` | 209715200 | 每用户原文总量 200 MiB |
| `XA_KB_MIN_PDF_CHARACTERS` | 50 | 每页平均有效字符下限 |
| `XA_KB_TARGET_TOKENS` / `XA_KB_MIN_TOKENS` | 500 / 50 | 切片目标与最小目标 |
| `XA_KB_OVERLAP_PERCENT` | 15 | 同章节或页内的重叠比例 |
| `XA_KB_EMBEDDING_BATCH_SIZE` | 20 | 每批向量条数，不能超过 20 |
| `XA_KB_TEXT_TYPE` | document | 摄取文本用途；兼容接口不支持时可显式设为空 |
| `XA_KB_JOB_TIMEOUT` | 10m | 单任务和上传提交的超时 |
| `XA_KB_ORPHAN_GRACE` | 24h | 孤儿原文宽限期，必须长于任务超时 |
| `XA_KB_CLEANUP_INTERVAL` | 1h | 孤儿原文检查间隔 |
| `XA_KB_TEMP_DIR` | ../.output/kb | 从 server 目录启动时的 PDF 临时目录 |

worker 必须能从 PATH 调用 `pdftotext`，Debian / Ubuntu 的包名是 `poppler-utils`。目前仓库只声明基础依赖容器，worker 是本机 Go 进程；打包 worker 镜像时必须安装相同依赖。PDF 临时文件仅在解析期间存在，不进入版本库。

## 后端验收

`TestKnowledgeR1` 创建独立 PostgreSQL 数据库，使用真实 Redis 会话、RustFS、百炼、Huma HTTP 接口及实际 worker 进程生命周期。PDF 路径通过 `XA_KB_TEST_TEXT_PDF`、`XA_KB_TEST_SCAN_PDF` 提供；文本 PDF 必须至少两页。测试结束清理数据库和该库前缀下的对象。

在启动脚本已加载运行配置、设置项目 `.output/` 临时目录后，在 `server/` 执行：

```bash
go test -race -tags=integration ./internal/rag -run TestKnowledgeR1 -v -count=1
go test -tags=integration ./internal/infra/provider -run TestBailianEmbeddingBatch -v -count=1
```

2026-11-13 开工核实：实际数据库的 pgvector 为 **0.8.7**；Poppler 为 **26.01.0**。百炼分别接受不传 `text_type`、`document`、`query` 的两条输入批次，均返回 2 × 1024 维。该验证证明参数被当前兼容接口接受，不据此推断两种用途的检索质量。

本地样本保存在独立 worktree 的 `.output/samples/`：`report.pdf` 是南京航空航天大学发布的 2025 年 3 月学习资料，共 70 页，`pdftotext` 抽出 59,390 个字符；`scan-page-1.pdf` 是中国政府网《国务院公报 2025·9》原 PDF 的首页，未抽出正文。另用 `testdata/chinese.md`、中文 TXT 和 4 页商家手册验证解析。大 PDF 不提交到 Git。

| 验收项 | 已观察到的结果 |
| --- | --- |
| 权限 | 普通用户创建公共库 403；其他用户和管理员访问、修改、删除私有库或列出其文档均 404；总列表不泄露私有库 |
| 配额与错误体 | 超文件大小 413；知识库数、文档数、用户空间超限拒绝；没有增加文档，失败上传对象已清理 |
| 去重 | 顺序与并发重复上传只有一份文档、一个活动摄取任务；重复响应 200 |
| 重试 | 已成功文档重新摄取遇到真实连接拒绝后为 failed；retry 后切片总数、唯一序号数与首次成功一致 |
| 解析 | 中文 TXT / Markdown ready；真实中文文本 PDF ready 且切片带页码；扫描件失败消息包含“扫描件，暂不支持 OCR”，第三次失败任务为 dead |
| 切片 | GET chunks 返回内容、Markdown 标题路径、字符范围；库内全文索引内容和 1024 维向量非空 |
| 删除与孤儿清理 | 删除后接口 404；delete 任务 completed；文档、切片及 S3 原文清空；清理孤儿保留仍被引用的原文 |
| worker | 现有 worker 同时启动记忆与知识循环，知识文档变为 ready，取消后正常退出 |
| 工程检查 | `make check` 通过：接口生成一致性、路由覆盖、gofmt、Go test / vet / build、两端 Biome / TypeScript / build |

## 边界与取舍

- R1 只验收创建、上传、解析、切片、分词、向量化、状态、去重、重试、删除和切片查询。不存在检索路由，也未启用 `kb_search`。
- 只支持 UTF-8 TXT / Markdown 和有文本层 PDF；不做 OCR、Word/PPT、网页抓取、复杂表格还原、rerank、父子切片、每轮预取或自动全量重建。`rebuild` 只保留后台任务类型，没有对外入口；未来可据版本差异标记需要重建。
- 保留现有 workspace 取值与 `configuration.knowledge[]` 形态，没有绑定表，没有绑定校验或新增权限。检索、引用、页面和质量评测按 [路线图](ROADMAP.md) 在 R2/R3 实现。
- 后台删除允许异步完成；删除任务三次失败后保留 dead 记录，由维护人员排查对象存储后重新安排。解析和向量化异常会使文档失败，不影响已有记忆和对话。
- 本批后端验收使用真实依赖，不做浏览器测试。前端更新接口生成物，并在已有请求封装中兼容 FormData；页面体验不属于本批验收范围。
