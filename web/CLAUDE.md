# Web 工程约定

使用 React 19、Umi 4、TypeScript、Tailwind CSS v4、Radix UI、Lucide 和 TanStack Table。根目录 AGENTS.md 与 CONTRIBUTING.md 同时适用。

- 使用 Node.js 22 或更新版本，使用 npm 和 package-lock.json。
- 界面沿用用户端的字体、颜色和圆角，通用控件保存在本项目的 `src/components/ui/`。
- 提交前运行 `npm run lint`、`npm run build` 和 `npm run test:smoke`。
- 仅使用 Biome 检查和格式化，不引入 ESLint 或 Prettier。
- `config/routes.ts` 管理路由，`src/components/AdminLayout.tsx` 管理布局和会话，`config/proxy.ts` 管理开发代理。
- `src/services/xa/` 统一处理请求和会话。浏览器验收使用真实后端，运行条件见 README.md。
- `src/.umi*` 由 Umi 自动生成，不手工修改，不提交。
- 添加必要的中文职责注释，保留后端权限检查和乐观锁，编辑基础字段不得删除未展示的配置。
- 按根目录规范验证并通过工作分支和 PR 提交。
