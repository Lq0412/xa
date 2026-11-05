# Web 工程约定

基于 Ant Design Pro v6.0.3，采用 React 19、Ant Design 6、ProComponents 3、Umi Max 4、TypeScript、antd-style 和 Tailwind CSS v4。根目录 AGENTS.md 与 CONTRIBUTING.md 同时适用。

- 使用 Node.js 22 或更新版本，使用 npm 和 package-lock.json。
- 布局使用 ProLayout，业务列表使用 ProTable，编辑使用 ProForm / ModalForm；优先沿用组件默认样式。
- 编写 antd 组件前运行 `npx antd info <Component> --format json` 核对 API；模板附带的 antd skill 位于 `.claude/skills/antd/SKILL.md`。
- 提交前运行 `npm run lint`、`npm run lint:antd` 和 `npm run build`；lint 和 antd 检查均需通过。
- 仅使用 Biome 检查和格式化，不引入 ESLint 或 Prettier。
- `config/routes.ts` 管理路由，`config/defaultSettings.ts` 管理布局，`config/proxy.ts` 管理开发代理。
- `src/services/xa/` 调用真实后端；`src/app.tsx` 恢复登录状态并配置请求。
- `src/services/ant-design-pro/` 和 `config/oneapi.json` 是上游生成代码与 schema，保留供模板参考，不得手工修改，不接入业务路由。
- Mock 已关闭。行为验证使用 `npm test` 连接本地真实 server、Redis 和 PostgreSQL，运行条件见 README.md。
- `src/.umi*` 由 Umi 自动生成，不手工修改，不提交。
- 添加必要的中文职责注释，保留后端权限检查和乐观锁，编辑基础字段不得删除未展示的配置。
- 本项目只保留 `main`，按根目录规范验证、提交并推送，不创建工作分支或 PR。
