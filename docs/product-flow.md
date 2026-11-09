> **已过时（2026-11-09）**：本文仍包含实时语音（Realtime）、文本/语音双通道切换、Flutter 移动端等已被现行蓝图裁掉的内容，不能再作为开发顺序或范围依据。
> 当前产品范围以 [ROADMAP.md](ROADMAP.md)、[blueprint.md](blueprint.md) 为准。
> Agent 工具循环的设计、分期和验收以 [agent-loop-design.md](agent-loop-design.md) 为准。
> 下文仅作历史记录保留。

# 顶层业务流程

## 1. 产品结构

系统由一个 Go server 支撑独立的 Web 用户端、Web 管理端和后续移动端：

~~~mermaid
flowchart LR
    Web[Web 管理端] --> Server[Go server]
    UserWeb[Web 用户端] --> Server
    Mobile[移动端：最后接入] --> Server
    Server --> Runtime[Agent Runtime]
~~~

Web 管理端负责平台资源和公共 Agent。独立 Web 用户端提供公开 Agent 列表、私有 Agent 创建和对话；先在 Web 完成全部能力和验收，最后开发移动端。

系统不为英语面试、IELTS、旅行等内容建立固定业务模块。它们都是 Agent 的名称、描述、提示词、工具和记忆配置。

## 2. 角色和权限

首期只有两类角色：

| 角色 | 主要入口 | 可以做什么 |
| --- | --- | --- |
| 管理员 | Web 管理端 | 管理平台模型、工具、知识库，创建和维护公共 Agent |
| 普通用户 | Web 用户端，后续接入移动端 | 使用公共 Agent，创建和维护自己的私有 Agent，进行对话 |

管理员创建的 Agent 可以对所有用户开放。普通用户创建的 Agent 默认只对创建者可见，首期不支持公开分享。

普通用户只能选择平台已经提供的模型、工具和知识库，不能新增供应商、修改平台凭证或修改公共 Agent。

## 3. Agent 的归属

每个 Agent 都有明确的创建者和可见范围：

| 字段 | 含义 |
| --- | --- |
| owner_type | admin 或 user |
| owner_id | 创建者身份 |
| visibility | public 或 private |
| current_version_id | 新会话使用的配置版本 |

公共 Agent 和私有 Agent 使用同一套配置、版本、会话、记忆和运行逻辑。归属和可见范围只影响谁能查看、修改和使用 Agent。

公共 Agent 不共享使用者的会话、录音、评价和记忆。普通用户只能读取自己的数据，管理员不因管理公共 Agent 而获得私人内容的浏览权限。

管理员可以设置一个公共 Agent 作为默认 Agent。默认 Agent 仍然是普通 Agent，不使用特殊运行逻辑。

## 4. Agent 生命周期

~~~mermaid
flowchart LR
    Create[创建 Agent] --> Configure[配置 Agent]
    Configure --> Validate[校验配置]
    Validate --> Save[保存]
    Save --> Active[立即可用]
    Active --> Update[修改配置]
    Update --> Configure
    Active --> Offline[下线]
~~~

创建或修改 Agent 时可以配置：

- 名称、头像、描述和标签。
- 提示词和首次问候语。
- 文本模型和 Realtime 模型。
- 可调用的工具。
- 可访问的知识库。
- 记忆策略和评价规则。
- 运行限制。

保存配置时进行校验。成功后生成不可变版本并更新 current_version_id，新会话使用该版本，已有会话继续使用创建时绑定的版本。修改已停用 Agent 不会自动启用；重新启用时再次校验资源。

停用或删除会取消当前回复和实时连接，历史仍可查看。旧版本不能绕过当前的停用、权限撤销或工具禁用。

## 5. 用户使用流程

### 5.1 选择 Agent

Web 用户端的 Agent 列表展示：

- 管理员创建的公共 Agent。
- 当前用户创建的私有 Agent。

用户可从列表创建私有 Agent，或选择现有 Agent 进入独立对话页。对话采用窄导航、会话列表、消息与输入区、可收起的 Agent 详情栏。功能完成后，创建的会话固定绑定当时的 Agent 版本，之后修改 Agent 配置不会改变这条会话的运行规则。当前对话布局只读取 Agent 资料，运行和历史仍待开发。

### 5.2 文本对话

~~~mermaid
sequenceDiagram
    participant U as 用户
    participant M as Web 用户端
    participant S as server
    participant R as Agent Runtime

    U->>M: 输入文本
    M->>S: 发送消息
    S->>R: 执行文本运行
    R-->>S: 文本和工具事件
    S-->>M: SSE 流式事件
    M-->>U: 展示回复
    S->>S: 保存消息和运行结果
~~~

文本通道使用 ReAct Agent。Agent 可以根据配置决定是否调用工具，工具结果返回后继续生成回复。

### 5.3 Realtime 对话

~~~mermaid
sequenceDiagram
    participant U as 用户
    participant M as Web 用户端
    participant S as server
    participant R as Realtime Runtime

    U->>M: 开始语音对话
    M->>S: 建立 WebSocket
    S->>R: 创建实时会话
    M->>S: 持续上传音频
    S->>R: 转发音频和控制事件
    R-->>S: 音频、转写、打断和回复事件
    S-->>M: WebSocket 事件
    M-->>U: 播放回复和展示转写
    S->>S: 保存转写、回复和会话结果
~~~

Realtime 通道和文本通道共用 Agent 配置、工具权限、会话记忆和评价任务，但拥有独立的连接生命周期和事件协议。

切换通道前先取消或结束当前执行并关闭原连接。同一会话只允许一个通道运行，使用已确认历史衔接上下文；语音中断后未播放的音频不能记为已交付回复。断线重连创建新的实时连接，不自动重放未确认音频。

## 6. 工具使用

工具由 Agent 配置决定。用户不需要选择运行模式，Agent 根据提示词、当前消息和工具描述自行判断是否调用工具。

工具调用必须经过：

~~~text
Agent 配置绑定
→ 服务端权限校验
→ 参数校验
→ 工具执行
→ 返回结构化结果
→ Agent 继续回复
~~~

用户创建私有 Agent 时，只能绑定管理员已经开放的工具。

## 7. 首期范围

首期顶层业务只要求完成：

~~~text
管理员创建公共 Agent
→ 保存配置并立即生效
→ 用户在 Web 用户端查看公共 Agent
→ 用户创建私有 Agent
→ 用户选择 Agent
→ 进行文本或 Realtime 对话
→ Agent 按配置调用工具
~~~

首期不包含：

- 用户 Agent 公开分享和 Agent 市场。
- 多管理员和团队协作。
- 用户自定义模型供应商。
- 固定的英语场景代码分支。
- 为不同 Agent 编写不同运行流程。
