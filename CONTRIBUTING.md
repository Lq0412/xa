# 贡献规范

## 分支

分支名称使用英文类型和英文短描述：

```text
feat/agent-runtime
fix/realtime-session
docs/architecture
```

常用类型：

```text
feat  新功能
fix   修复问题
docs  文档修改
refactor  重构
test  测试
chore  工程维护
```

## 提交信息

提交信息使用 Conventional Commits 格式，类型和作用域保持英文，标题和正文使用中文：

```text
<type>(<scope>): <中文标题>
```

示例：

```text
docs(product): 补充顶层 Agent 业务流程
feat(agent): 增加私有 Agent 创建能力
fix(runtime): 修正实时会话关闭处理
```

需要补充说明时，正文使用中文，说明修改原因、主要变化和验证结果：

```text
docs(product): 补充顶层 Agent 业务流程

明确管理员公共 Agent、用户私有 Agent 和两种客户端的职责边界。
补充文本对话与 Realtime 对话的顶层流程。

验证：人工检查文档结构。
```

提交信息不添加 `Co-authored-by` 等自动签名。

## 推送和合并

1. 从 `main` 创建短分支。
2. 在分支上完成修改和验证。
3. 提交信息遵循本文件的格式。
4. 推送分支并创建 PR。
5. PR 标题沿用提交格式，PR 正文使用中文。
6. 等待检查和 Review 完成后再合并。

禁止直接向受保护的 `main` 分支推送。

## 验证

提交前运行与修改范围对应的检查命令，并在 PR 正文中写明实际执行的命令和结果。

Go 后端至少运行：

```bash
cd server
go test ./...
go vet ./...
go build ./...
```

## PR 内容

PR 正文使用中文，至少包含：

- 功能变化。
- 实现方式。
- 实际验证命令和结果。
- 关联的 Issue（如果有）。
- 涉及界面变化时附截图或录屏。
