# 接口约定

Go 输入输出结构体和 Huma operation 是接口定义的来源；[openapi.yaml](openapi.yaml) 由代码生成，禁止手改。两个前端的 `src/services/generated/` 都从这一份 OpenAPI 3.1 文件生成。

在仓库根目录使用 Node.js 24：

```bash
nvm use
make api
make api-client
make api-docs
```

`make api` 不读取运行配置、不连接数据库或 Redis，也不启动 HTTP 服务。`make api-client` 同时更新 YAML 和两个客户端；用户端使用 `max openapi`，管理端使用独立的 `@umijs/openapi`，没有引入 Umi Max。两端生成器共用对 OpenAPI 3.1 可空类型的兼容转换，原 YAML 保持不变。

`make api-docs` 在 <http://localhost:5177/> 启动支持搜索和文件监听的 Scalar 页面。首次使用需要联网下载固定版本的 CLI，文档服务不挂到业务服务器上。

修改接口后重新生成，审阅并暂存生成文件，再执行 `make check`。检查会重新生成并用 `git diff` 比对工作区与暂存内容，同时拒绝未纳入版本控制的生成文件；路由覆盖测试比较实际注册和文档中的方法、路径。它们不验证真实运行中的响应体，本阶段不做集成响应校验。

两处 SSE 沿用原 `net/http` handler、事件帧和手写流客户端，OpenAPI 只声明协议与输入输出形状。工具产生的动态 JSON 保留开放结构。[memory.md](memory.md) 保留记忆业务规则，接口字段以生成的约定为准。
