declare namespace API {
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

  type ApiError = {
    /** 稳定的错误码 */
    code: string;
    /** 错误说明 */
    message: string;
    /** 请求标识 */
    request_id: string;
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

  type CredentialsRequest = {
    /** 注册时使用的显示名称 */
    display_name?: string;
    /** 账号邮箱 */
    email: string;
    /** 账号密码 */
    password: string;
  };

  type deleteAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type enableAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type EvaluationBinding = {
    enabled: boolean;
    rubric_id: string;
  };

  type getAgentConfigParams = {
    /** Agent ID */
    agent_id: string;
  };

  type getAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type KnowledgeBinding = {
    enabled: boolean;
    filter: string;
    knowledge_base_id: string;
  };

  type LoginResponse = {
    /** 会话 Bearer 令牌 */
    token: string;
    /** 当前用户 */
    user: UserResponse;
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

  type saveAgentParams = {
    /** Agent ID */
    agent_id: string;
  };

  type Slot = {
    content: string;
    enabled: boolean;
    name: string;
    order: number;
    scope: string;
    variables: string[] | null;
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
}
