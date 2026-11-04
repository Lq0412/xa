declare module '*.css';
declare module '*.less';
declare module '*.scss';
declare module '*.sass';
declare module '*.svg';
declare module '*.png';
declare module '*.jpg';
declare module '*.jpeg';
declare module '*.gif';
declare module '*.bmp';
declare module '*.tiff';
declare module '*.md' {
  const content: string;
  export default content;
}

declare namespace XA {
  /** 服务端认证后的用户身份，客户端不得自行指定角色。 */
  type User = {
    id: string;
    workspace_id: string;
    email: string;
    display_name: string;
    role: 'user' | 'admin';
  };

  /** Agent 的稳定身份和当前可用状态。 */
  type Agent = {
    id: string;
    workspace_id: string;
    owner_id: string;
    owner_type: 'user' | 'system';
    current_version_id: string;
    name: string;
    avatar: string;
    description: string;
    tags: string[] | null;
    visibility: 'public' | 'private';
    status: 'active' | 'offline' | 'deleted';
    is_default: boolean;
    created_by: string;
    created_at: string;
    updated_at: string;
    deleted_at?: string;
  };

  /** 独立提示词片段，编辑主提示词时保留其他片段。 */
  type PromptSlot = {
    name: string;
    content: string;
    enabled: boolean;
    order: number;
    variables: string[];
    scope: string;
  };

  /** 服务端完整配置；尚未提供编辑界面的字段仍需原样提交。 */
  type Configuration = {
    prompts: PromptSlot[] | null;
    models?: unknown[] | null;
    tools?: unknown[] | null;
    knowledge?: unknown[] | null;
    evaluations?: unknown[] | null;
    memory_policy?: unknown;
    runtime_limits?: unknown;
  };

  /** 不可变配置版本，保存时携带其 id 进行并发校验。 */
  type AgentVersion = {
    id: string;
    agent_id: string;
    version_number: number;
    configuration: Configuration;
    config_checksum: string;
    created_at: string;
  };

  /** 登录响应同时返回令牌与已核验用户。 */
  type LoginResult = { token: string; user: User };
}
