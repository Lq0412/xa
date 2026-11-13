declare namespace API {
  type ActiveRun = {
    id: string;
    last_event_seq: number;
    pending_approvals?: Approval[] | null;
    status: string;
  };

  type ActivityItem = {
    created_at: string;
    id: string;
    result: string;
    title: string;
  };

  type ActivityPage = {
    items: ActivityItem[];
    next_cursor: string;
  };

  type addAgentToLibraryParams = {
    /** Agent ID */
    agent_id: string;
  };

  type AddMemoryRequest = {
    /** Agent 范围对应的 ID */
    agent_id?: string;
    /** 记忆内容 */
    content: string;
    /** 重要性，取值 1 至 10 */
    importance: number;
    /** 记忆类型，默认 fact */
    kind?: "fact" | "preference" | "episode" | "procedure";
    /** 命名空间 */
    namespace: string;
    /** 是否置顶 */
    pinned?: boolean;
    /** 记忆范围，默认 user */
    scope?: "user" | "agent";
    /** 主题键，省略时自动生成 */
    subject_key?: string;
  };

  type AgentConfigResponse = {
    agent_id: string;
    /** 可选择的模型 */
    available_models: string[];
    config_checksum: string;
    configuration: Configuration;
    created_at: string;
    created_by: string;
    id: string;
    version_number: number;
    workspace_id: string;
  };

  type AgentDefinition = {
    avatar: string;
    created_at: string;
    created_by: string;
    current_version_id: string;
    deleted_at?: string;
    description: string;
    id: string;
    is_default: boolean;
    is_primary: boolean;
    name: string;
    owner_id: string;
    owner_type: string;
    owner_user_id?: string;
    status: string;
    tags: string[] | null;
    updated_at: string;
    visibility: "public" | "private";
    workspace_id: string;
  };

  type AgentListResponse = {
    /** 符合当前访问条件的 Agent */
    agents: AgentDefinition[] | null;
  };

  type AgentRequest = {
    /** 兼容旧客户端，服务端以请求路径为准 */
    agent_id?: string;
    /** 头像 */
    avatar?: string;
    /** 运行配置，省略字段由 Agent 服务处理 */
    configuration?: {
      enable_thinking?: boolean;
      evaluations?: EvaluationBinding[] | null;
      knowledge?: KnowledgeBinding[] | null;
      memory_policy?: any;
      model?: string;
      models?: ModelBinding[] | null;
      prompts?: Slot[] | null;
      runtime_limits?: any;
      soul_md?: string;
      tools?: ToolBinding[] | null;
    };
    /** 描述 */
    description?: string;
    /** 更新时用于并发校验的当前版本 ID */
    expected_version_id?: string;
    /** Agent 名称 */
    name: string;
    /** 标签 */
    tags?: string[] | null;
    /** 可见范围 */
    visibility: "public" | "private";
  };

  type AgentSaveResponse = {
    /** 保存后的 Agent */
    agent: AgentDefinition;
    /** 配置校验值 */
    config_checksum: string;
    /** 当前配置版本 ID */
    current_version_id: string;
    /** 配置版本序号 */
    version_number: number;
  };

  type AgentToolEntry = {
    approval: "never" | "always";
    approval_overridable: boolean;
    available: boolean;
    /** 工具特定的生效配置 */
    configuration: any;
    enabled: boolean;
    max_calls: number;
    name: string;
    reason: string;
    timeout_ms: number;
    title: string;
  };

  type AgentToolsResponse = {
    current_version_id: string;
    runtime_limits: RuntimeLimits;
    tools: AgentToolEntry[];
    toolset_hash: string;
    /** 更新后返回的新版本序号 */
    version_number?: number;
  };

  type ApiError = {
    /** 稳定的错误码 */
    code: string;
    /** 错误说明 */
    message: string;
    /** 请求标识 */
    request_id: string;
  };

  type Approval = {
    approval_id: string;
    args_preview?: string;
    call_id: string;
    can_remember_agent: boolean;
    expires_at: string;
    items?: DisplayItem[] | null;
    name: string;
    reason?: string;
    remember?: string;
    run_id: string;
    status: string;
    summary: string;
    title: string;
  };

  type ApprovalRecord = {
    agent: string;
    agent_id: string;
    conversation_id: string;
    created_at: string;
    decider: string;
    id: string;
    remember: string;
    run_id: string;
    status: string;
    summary: any;
    title: any;
    tool_name: string;
    user: string;
    wait_ms: number;
  };

  type ApprovalRecordsPage = {
    approvals: ApprovalRecord[];
    next_cursor: string;
  };

  type Base = {
    chunk_count: number;
    chunker_version: string;
    created_at: string;
    description: string;
    document_count: number;
    embed_model: string;
    id: string;
    name: string;
    owner_user_id: string;
    updated_at: string;
    visibility: "private" | "public";
    workspace_id: string;
  };

  type BasePage = {
    items: Base[] | null;
    total: number;
  };

  type CancelResult = {
    status: string;
  };

  type cancelRunParams = {
    /** 运行 ID */
    run_id: string;
  };

  type Chunk = {
    /** 规范化文本中的 Unicode 字符终点，不包含此位置 */
    char_end: number;
    /** 规范化文本中的 Unicode 字符起点，包含此位置 */
    char_start: number;
    chunk_index: number;
    content: string;
    document_id: string;
    embed_model: string;
    heading_path: string[] | null;
    id: string;
    knowledge_base_id: string;
    page_from: number | null;
    page_to: number | null;
    /** 切片估算 token 数 */
    token_count: number;
  };

  type ChunkPage = {
    items: Chunk[] | null;
    total: number;
  };

  type Configuration = {
    enable_thinking?: boolean;
    evaluations: EvaluationBinding[] | null;
    knowledge: KnowledgeBinding[] | null;
    memory_policy: any;
    model: string;
    models: ModelBinding[] | null;
    prompts?: Slot[] | null;
    runtime_limits: any;
    soul_md: string;
    tools: ToolBinding[] | null;
  };

  type Conversation = {
    agent_id: string;
    created_at: string;
    id: string;
    title: string;
    updated_at: string;
    user_id: string;
    workspace_id: string;
  };

  type ConversationList = {
    conversations: Conversation[];
  };

  type CreateInput = {
    description?: string;
    name: string;
    visibility?: "private" | "public";
  };

  type CredentialsRequest = {
    /** 注册时使用的显示名称 */
    display_name?: string;
    /** 账号邮箱 */
    email: string;
    /** 账号密码 */
    password: string;
  };

  type DeadJob = {
    id: string;
    kind: string;
    last_error_summary: string;
    updated_at: string;
  };

  type decideApprovalParams = {
    /** 审批 ID */
    approval_id: string;
  };

  type DecisionBody = {
    decision: "approve" | "reject";
    reason?: string;
    remember?: "once" | "conversation" | "agent";
  };

  type DecisionResult = {
    approval_id: string;
    run_id: string;
    run_status: string;
    status: string;
    warning?: string;
  };

  type deleteAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type deleteKnowledgeBaseParams = {
    id: string;
  };

  type deleteKnowledgeDocumentParams = {
    id: string;
    docId: string;
  };

  type deleteMemoryParams = {
    /** 记忆条目 ID */
    memory_id: string;
  };

  type DisplayItem = {
    label: string;
    value: string;
  };

  type Document = {
    chunk_count: number;
    chunker_version: string;
    content_sha256: string;
    created_at: string;
    embed_model: string;
    /** 摄取失败码：unsupported_file_type、invalid_file_type、invalid_utf8、file_too_large、empty_document、too_many_pages、too_many_characters、scanned_pdf、pdf_parse_failed、source_unavailable、embedding_failed、ingest_failed、delete_failed */
    error_code: string;
    error_message: string;
    filename: string;
    id: string;
    knowledge_base_id: string;
    mime: string;
    owner_user_id: string;
    page_count: number;
    ready_at: string | null;
    size_bytes: number;
    status: "pending" | "processing" | "ready" | "failed" | "deleting";
    title: string;
    token_count: number;
    updated_at: string;
    workspace_id: string;
  };

  type DocumentPage = {
    items: Document[] | null;
    total: number;
  };

  type EditInput = {
    content: string;
    importance: number;
    namespace: string;
    pinned: boolean;
  };

  type editMemoryParams = {
    /** 记忆条目 ID */
    memory_id: string;
  };

  type enableAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type EvaluationBinding = {
    enabled: boolean;
    rubric_id: string;
  };

  type Event = {
    actor_id: string;
    actor_type: string;
    after: any;
    before: any;
    created_at: string;
    id: number;
    memory_id: string | null;
    op: string;
    reason: string;
    source_ref: string | null;
  };

  type getAdminRunParams = {
    /** 运行 ID */
    run_id: string;
  };

  type getAdminRunSpansParams = {
    /** 运行 ID */
    run_id: string;
  };

  type getAgentActivityParams = {
    /** 每页条数，范围 1 至 100，默认 20 */
    limit?: string;
    /** 上一页返回的 next_cursor */
    cursor?: string;
    /** Agent ID */
    agent_id: string;
  };

  type getAgentConfigParams = {
    /** Agent ID */
    agent_id: string;
  };

  type getAgentMemoryParams = {
    /** Agent ID */
    agent_id: string;
  };

  type getAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type getAgentToolsParams = {
    /** Agent ID */
    agent_id: string;
  };

  type getConversationSummaryParams = {
    /** 会话 ID */
    conversation_id: string;
  };

  type getKnowledgeBaseParams = {
    id: string;
  };

  type getKnowledgeDocumentParams = {
    id: string;
    docId: string;
  };

  type getRunParams = {
    /** 运行 ID */
    run_id: string;
  };

  type getRunSpansParams = {
    /** 运行 ID */
    run_id: string;
  };

  type getToolResultParams = {
    /** 运行 ID */
    run_id: string;
    /** 工具调用 ID */
    call_id: string;
  };

  type HealthLive = {
    /** 进程状态 */
    status: "ok";
  };

  type HealthReady = {
    /** 各依赖的状态 */
    dependencies: Record<string, any>;
    /** 依赖就绪状态 */
    status: "ok" | "unavailable";
  };

  type Item = {
    agent_id: string | null;
    content: string;
    conversation_id: string | null;
    created_at: string;
    id: string;
    importance: number;
    kind: string;
    last_used_at: string | null;
    namespace: string;
    owner_user_id: string;
    pinned: boolean;
    retention: number;
    scope: string;
    sensitive: boolean;
    source: string;
    source_ref: string | null;
    source_type: string;
    status: string;
    strength_days: number;
    subject_key: string;
    supersedes_id: string | null;
    updated_at: string;
    workspace_id: string;
  };

  type KnowledgeBinding = {
    enabled: boolean;
    /** 保留字段，目前必须为空；不支持过滤表达式 */
    filter: string;
    /** 知识库 ID；保存时校验所有权及可见性，检索时再次检查当前权限 */
    knowledge_base_id: string;
  };

  type LibraryListResponse = {
    /** 当前用户库中可对话的 Agent */
    agents: AgentDefinition[];
  };

  type listAdminApprovalsParams = {
    /** 按审批状态筛选 */
    status?: "pending" | "approved" | "rejected" | "expired" | "cancelled";
    /** 按工具名称筛选 */
    tool_name?: string;
    /** 上一页返回的 next_cursor */
    cursor?: string;
    /** 每页条数，范围 1 至 100，默认 50 */
    limit?: string;
  };

  type listAdminRunsParams = {
    /** 按会话 ID 筛选 */
    conversation_id?: string;
    /** 按 Agent ID 筛选 */
    agent_id?: string;
    /** 按运行状态筛选 */
    status?:
      | "queued"
      | "running"
      | "waiting_approval"
      | "completed"
      | "failed"
      | "cancelled"
      | "budget_exhausted"
      | "interrupted";
    /** 按用户 ID 筛选，仅管理员可用 */
    user_id?: string;
    /** 上一页返回的 next_cursor */
    cursor?: string;
    /** 每页条数，范围 1 至 100，默认 20 */
    limit?: string;
  };

  type listConversationMessagesParams = {
    /** 会话 ID */
    conversation_id: string;
    /** 从此消息之前开始读取 */
    before?: string;
    /** 每页条数，范围 1 至 100，默认 50 */
    limit?: string;
  };

  type listKnowledgeBasesParams = {
    limit?: number;
    offset?: number;
  };

  type listKnowledgeChunksParams = {
    id: string;
    docId: string;
    limit?: number;
    offset?: number;
  };

  type listKnowledgeDocumentsParams = {
    id: string;
    limit?: number;
    offset?: number;
  };

  type listMemoriesParams = {
    /** 记忆范围 */
    scope?: "user" | "agent" | "session" | "all_visible";
    /** 筛选指定 Agent */
    agent_id?: string;
    /** 筛选命名空间 */
    namespace?: string;
    /** 记忆状态，默认 active */
    status?: "active" | "deleted" | "superseded" | "archived";
    /** 每页条数，默认 50，超过 200 按 200 处理 */
    limit?: number;
    /** 上页返回的游标 */
    cursor?: string;
  };

  type listMemoryEventsParams = {
    /** 记忆条目 ID */
    memory_id: string;
  };

  type listRunsParams = {
    /** 按会话 ID 筛选 */
    conversation_id?: string;
    /** 按 Agent ID 筛选 */
    agent_id?: string;
    /** 按运行状态筛选 */
    status?:
      | "queued"
      | "running"
      | "waiting_approval"
      | "completed"
      | "failed"
      | "cancelled"
      | "budget_exhausted"
      | "interrupted";
    /** 按用户 ID 筛选，仅管理员可用 */
    user_id?: string;
    /** 上一页返回的 next_cursor */
    cursor?: string;
    /** 每页条数，范围 1 至 100，默认 20 */
    limit?: string;
  };

  type listUserApprovalsParams = {
    /** 每页条数，范围 1 至 100，默认 20 */
    limit?: string;
    /** 上一页返回的 next_cursor */
    cursor?: string;
    /** 按 Agent ID 筛选 */
    agent_id?: string;
  };

  type LoginResponse = {
    /** 会话 Bearer 令牌 */
    token: string;
    /** 当前用户 */
    user: UserResponse;
  };

  type MemoryEventsResponse = {
    /** 记忆取代链上的审计事件 */
    events: Event[] | null;
  };

  type MemoryPolicy = {
    core_budget_tokens: number;
    decay_enabled: boolean;
    enabled: boolean;
    history_budget_tokens: number;
    min_relevance: number;
    recall_top_k: number;
    summary_budget_tokens: number;
    summary_trigger_tokens: number;
  };

  type MemoryPolicyRequest = {
    core_budget_tokens?: number;
    decay_enabled?: boolean;
    /** 是否启用记忆 */
    enabled: boolean;
    /** 当前 Agent 配置版本 ID */
    expected_version_id: string;
    history_budget_tokens?: number;
    min_relevance?: number;
    recall_top_k?: number;
    summary_budget_tokens?: number;
    summary_trigger_tokens?: number;
  };

  type MemoryPolicyResponse = {
    current_version_id: string;
    memory_policy: MemoryPolicy;
    version_number: number;
  };

  type Message = {
    completion_tokens: number;
    content: string;
    conversation_id: string;
    created_at: string;
    error?: string;
    id: string;
    latency_ms: number;
    model: string;
    parts?: Record<string, any>[] | null;
    prompt_tokens: number;
    role: "system" | "user" | "assistant";
    run_id?: string;
    status: "pending" | "completed" | "failed";
  };

  type MessagePage = {
    active_run?: ActiveRun;
    before?: string;
    has_more: boolean;
    messages: Message[];
  };

  type ModelBinding = {
    capability: string;
    model_profile_id: string;
    revision: string;
  };

  type offlineAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type Page = {
    items: Item[] | null;
    next_cursor: string;
  };

  type restoreMemoryParams = {
    /** 记忆条目 ID */
    memory_id: string;
  };

  type retryKnowledgeDocumentParams = {
    id: string;
    docId: string;
  };

  type retryMemoryJobParams = {
    /** 记忆任务 ID */
    job_id: string;
  };

  type RunDetail = {
    active_ms: number;
    agent_id: string;
    agent_name: string;
    agent_version_id: string;
    cached_tokens: number;
    cancel_reason: string;
    completion_tokens: number;
    conversation_id: string;
    created_at: string;
    ended_at: string | null;
    error: string;
    error_code: string;
    final_message_id: string;
    id: string;
    latency_ms: number;
    limits: any;
    model: string;
    owner_user_id: string;
    prompt_tokens: number;
    provider: string;
    started_at: string | null;
    status: string;
    steps: number;
    stream_protocol: number;
    tool_calls: number;
    toolset: (string | null)[];
    toolset_hash: string;
    ttft_ms: number | null;
    user_email?: string;
    user_message_id: string;
    workspace_id: string;
  };

  type RunList = {
    next_cursor: string;
    runs: RunSummary[];
  };

  type RunSummary = {
    active_ms: number;
    agent_id: string;
    agent_name: string;
    cached_tokens: number;
    completion_tokens: number;
    conversation_id: string;
    created_at: string;
    ended_at: string | null;
    error_code: string;
    id: string;
    latency_ms: number;
    model: string;
    prompt_tokens: number;
    started_at: string | null;
    status: string;
    steps: number;
    tool_calls: number;
    ttft_ms: number | null;
    user_email?: string;
  };

  type RuntimeLimits = {
    history_tool_runs: number;
    max_calls_per_step: number;
    max_model_turns: number;
    max_output_tokens: number;
    max_parallel_tools: number;
    max_tool_calls: number;
    max_total_tokens: number;
    max_wall_time_ms: number;
    tool_result_max_tokens: number;
  };

  type saveAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type sendConversationMessageParams = {
    /** 会话 ID */
    conversation_id: string;
    /** 传 2 选择 v2 事件协议 */
    stream?: string;
  };

  type SendMessageBody = {
    /** 用户消息正文 */
    content: string;
  };

  type Slot = {
    content: string;
    enabled: boolean;
    name: string;
    order: number;
    scope: string;
    variables: string[] | null;
  };

  type SpanNode = {
    attributes: Record<string, any>;
    cached_tokens: number | null;
    call_id: string;
    children: SpanNode[];
    completion_tokens: number | null;
    duration_ms: number | null;
    ended_at: string | null;
    error_code: string;
    estimated_tokens: number | null;
    id: string;
    input_preview: string;
    kind: string;
    name: string;
    output_preview: string;
    parent_span_id?: string;
    prompt_tokens: number | null;
    run_id: string;
    started_at: string;
    status: string;
    step: number | null;
  };

  type Stats = {
    dead: number;
    dead_jobs: DeadJob[] | null;
    failed: number;
    latest_maintain_at?: string;
    latest_summary_at?: string;
    namespace: Record<string, any>;
    pending: number;
    status: Record<string, any>;
  };

  type StreamDelta = {
    delta: string;
  };

  type StreamDone = {
    completion_tokens: number;
    message_id: string;
    prompt_tokens: number;
  };

  type StreamError = {
    message: string;
  };

  type subscribeRunEventsParams = {
    /** 运行 ID */
    run_id: string;
    /** 从此非负序号之后开始读取，默认 0 */
    after?: number;
  };

  type Summary = {
    content: string;
    conversation_id: string;
    covers_from_message_id: string | null;
    covers_until_message_id: string | null;
    created_at: string;
    estimated_tokens: number;
    id: string;
    message_count: number;
    version: number;
  };

  type ToolBinding = {
    allowed_roles?: string[] | null;
    approval?: string;
    configuration?: any;
    enabled: boolean;
    max_calls?: number;
    name: string;
    timeout_ms?: number;
    tool_version_id?: string;
  };

  type ToolCatalogApproval = {
    default: "never" | "always";
    overridable: boolean;
  };

  type ToolCatalogItem = {
    approval: ToolCatalogApproval;
    available: boolean;
    /** 工具特定的配置 JSON Schema */
    config_schema: any;
    default_enabled: boolean;
    description: string;
    effect: "read" | "write" | "destructive";
    /** 工具特定的输入 JSON Schema */
    input_schema: any;
    name: string;
    title: string;
  };

  type ToolCatalogResponse = {
    tools: ToolCatalogItem[];
  };

  type ToolResultDetail = {
    arguments: any;
    call_id: string;
    data: any;
    display: any;
    model_content: string;
    run_id: string;
    status: string;
    tool_name: string;
  };

  type updateAgentToolsParams = {
    /** Agent ID */
    agent_id: string;
  };

  type UpdateAgentToolsRequest = {
    /** 当前 Agent 配置版本 ID */
    expected_version_id: string;
    /** 运行预算，省略时保留当前预算 */
    runtime_limits?: {
      history_tool_runs?: number;
      max_calls_per_step?: number;
      max_model_turns?: number;
      max_output_tokens?: number;
      max_parallel_tools?: number;
      max_tool_calls?: number;
      max_total_tokens?: number;
      max_wall_time_ms?: number;
      tool_result_max_tokens?: number;
    } | null;
    /** Agent 的工具绑定 */
    tools: ToolBinding[] | null;
  };

  type UpdateInput = {
    description?: string;
    name?: string;
  };

  type updateKnowledgeBaseParams = {
    id: string;
  };

  type updateMemoryPolicyParams = {
    /** Agent ID */
    agent_id: string;
  };

  type uploadKnowledgeDocumentParams = {
    id: string;
  };

  type UserApproval = {
    agent: string;
    agent_id: string;
    conversation_id: string;
    created_at: string;
    decided_at: string | null;
    id: string;
    remember: string;
    run_id: string;
    status: string;
    summary: string | null;
    title: string | null;
    tool_name: string;
    wait_ms: number;
  };

  type UserApprovalPage = {
    approvals: UserApproval[];
    next_cursor: string;
  };

  type UserResponse = {
    /** 显示名称 */
    display_name: string;
    /** 账号邮箱 */
    email: string;
    /** 用户 ID */
    id: string;
    /** 用户角色 */
    role: string;
    /** 工作空间 ID */
    workspace_id: string;
  };

  type View = {
    estimated_tokens: number;
    item_ids: string[] | null;
    markdown: string;
  };

  type WaitingApprovalResponse = {
    code: "run_waiting_approval";
    message: string;
    run_id: string;
  };
}
