# Ant Design Pro v6 管理端验收

前端基于 Ant Design Pro v6.0.3 发布模板，使用 React 19.2.7、Ant Design 6.6.0、Umi Max 4.7.3 和 ProComponents 3.1.14-2。工作分支为 `feat/web-agent-console`。

## 可验收内容

访问 `http://127.0.0.1:8000`，使用已有账号登录或注册普通账号：

- 标准 ProLayout 侧栏、账号菜单和 ProTable 列表；按名称、范围、状态查询。
- 新建、编辑和只读查看使用 ModalForm。普通用户只能维护自己的私有 Agent。
- 保存时保留其他提示词、完整配置及头像；配置版本过期时提示重新读取。
- 启用、停用、确认删除及退出登录调用真实后端；刷新后恢复 Redis 会话。

## 已执行验证

| 命令 | 验证结果 |
| --- | --- |
| `make check` | Go 测试、vet、build，以及 Web 的 Biome、TypeScript、antd 检查和构建通过 |
| `cd web && npm test` | 两条 Chromium 浏览器用例通过，使用真实 server、PostgreSQL 和 Redis |
| `curl --fail http://127.0.0.1:8000/health/ready` | Web 代理成功，三个基础依赖均就绪 |

浏览器用例覆盖登录、刷新会话、创建、筛选、编辑后未展示配置的保留、真实并发保存返回 409、启停、删除、退出后令牌失效、错误密码、重复注册、过期会话、404 和窄屏登录页。测试仅删除自己创建的 Agent，并撤销测试会话；没有替换网络响应。

本机 Chromium 及缺失的动态库放在 `.output/browsers/` 和 `.output/browser-libs/`。执行测试时通过 `PLAYWRIGHT_BROWSERS_PATH`、`LD_LIBRARY_PATH`、`TMPDIR` 指向这些本地目录，账号通过 `XA_E2E_EMAIL`、`XA_E2E_PASSWORD` 注入。测试输出为 `.output/web-e2e/`。

## 当前边界

本次更换管理界面并接回已有接口。管理员公共 Agent 操作依赖管理员配置，本轮浏览器用例使用普通测试账号；移动端、邮件验证、模型接入、文本对话和 Realtime 尚未实现。本阶段不表示 Agent 已能运行对话。
