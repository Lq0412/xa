# 管理端工程约定

同时遵循[项目规范](../AGENTS.md)和[贡献规范](../CONTRIBUTING.md)。

- 界面以 Linear 和 Apple Notes 的简洁为参考。删除不必要的说明和次要标签，沿用已确认的字体、颜色、圆角和间距。表格不加外框，用浅分隔线区分行，状态切换放标题下，其他筛选放标题行右侧。
- 使用 Node.js 24、npm、Umi 4、Tailwind CSS 4、Radix UI、Lucide 和 TanStack Table。通用控件放在本项目的 `src/components/ui/`，格式检查使用 Biome。
- 路由由 `config/routes.ts` 管理，开发代理见 `config/proxy.ts`。请求和会话沿用 `src/services/xa/`，生成客户端按根目录接口约定维护。Umi 生成的 `src/.umi*` 不手工修改、不提交。
- 保留服务端权限与配置版本校验。编辑基础字段时保留未展示的配置。
- 前端代码改动运行 `npm run lint` 和 `npm run build`。涉及登录、导航、配置保存等交互时，用真实后端运行相关 `npm run test:smoke` 场景。配置保存后核对重新读取的值。
