# xa Web

使用 Umi 4、React 19、Tailwind CSS 4、Radix UI、Lucide 和 TanStack Table。管理员登录后可以访问今天、Agent 列表、整页配置和只读确认记录。普通用户使用独立的 `web-user/` 工程。

## 启动

需要 Node.js 22 或更新版本。先按 [启动说明](../docs/startup.md) 启动基础依赖和 server，再在仓库根目录运行：

```bash
make web-install
make web
```

访问 `http://localhost:8000`。开发代理默认连接 `http://127.0.0.1:18080`，可通过 `XA_API_PROXY` 修改。管理员使用部署配置中的账号登录，普通用户注册与使用入口在独立用户端。

## 开发与验证

```bash
cd web
npm ci
npm run lint
npm run build
```

`config/` 管理路由和开发代理，`src/pages/` 存放业务页面，`src/components/ui/` 存放本项目的通用控件，`src/services/xa/` 封装真实接口。生成目录 `src/.umi*`、依赖和构建产物不提交。

## 浏览器验收

`npm run test:smoke` 使用 Playwright 连接已运行的真实后端，验证登录、今天、Agent 创建与配置保存、工具审批切换、未保存退出保护、版本冲突、启停、确认记录及退出。临时 Agent 和测试会话在结束后清理。先准备 Chromium，并通过环境变量 `XA_E2E_EMAIL`、`XA_E2E_PASSWORD` 注入已有的管理员测试账号。管理端默认访问 `http://127.0.0.1:8000`，可用 `XA_WEB_URL` 修改。可选的 `XA_ADMIN_SHOTS` 指定截图目录，登录页截图在输入凭证前采集。

`npm test` 还会运行原有的两端回归，使用 `XA_USER_WEB_URL` 指定用户端地址，默认 `http://127.0.0.1:8002`。测试输出保存在仓库根目录 `.output/web-e2e/`。

## 构建产物

`npm run build` 输出 `dist/`，开发资源使用独立目录。安装时 `prepare` 生成 Umi 类型。

生产静态服务器需要将页面路由回退到 `index.html`，并将 `/api/` 和 `/health/` 代理到 server。构建时通过 `XA_USER_WEB_URL` 指定用户端地址。

今天页遍历审批接口的全部游标后计算统计；“今日已处理”和平均等待按本机自然日内的决定计算，包含拒绝、超时和取消。确认记录只展示服务端已提供的脱敏摘要与字段，不推断原始参数和到期时间。运行追踪入口暂不可用。

Agent 的 SOUL、模型和工具通过完整配置版本保存，保留知识库、评价、记忆策略、运行限制及其他未编辑的绑定。新建时模型使用服务端默认值，创建后可选择服务端返回的模型。记忆和运行限制目前只读。
