# 用户端工程约定

同时遵循[项目规范](../AGENTS.md)和[贡献规范](../CONTRIBUTING.md)。

- 界面以 Linear 和 Apple Notes 的简洁为参考。对话与内容优先，沿用已确认的字体、颜色、圆角和间距，删去不必要的说明、标签、卡片和边框。
- 交互改动检查适用的窄屏布局、键盘操作和焦点状态。审批、停止和恢复要反映服务端状态。
- 使用 Node.js 24、npm、Umi Max 4、Tailwind CSS 4、Radix UI 和 Lucide。通用控件放在本项目的 `src/components/ui/`，格式检查使用 Biome。
- 接口调用沿用 `src/services/`，生成客户端按根目录接口约定维护。Umi 生成的 `src/.umi*` 不手工修改、不提交。
- 保留服务端权限与配置版本校验。编辑时保留未展示的配置。
- 前端代码改动运行 `npm run lint` 和 `npm run build`。逻辑改动运行相关既有单元测试，入口为 `npm run test:unit`。对话、审批、停止、恢复、权限或配置保存改动，用真实后端运行相关 Playwright 场景，入口为 `npm test`。
