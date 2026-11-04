// 用户身份来自 server 的认证接口，页面不自行推断角色。
export type User = {
  id: string
  workspace_id: string
  email: string
  display_name: string
  role: string
}

// AgentDefinition 是管理列表中可见的稳定身份。
export type Agent = {
  id: string
  workspace_id: string
  owner_id: string
  owner_type: string
  current_version_id: string
  name: string
  avatar: string
  description: string
  tags: string[] | null
  visibility: 'public' | 'private'
  status: 'active' | 'offline' | 'deleted'
  is_default: boolean
  created_by: string
  created_at: string
  updated_at: string
  deleted_at?: string
}

export type PromptSlot = {
  name: string
  content: string
  enabled: boolean
  order: number
  variables: string[]
  scope: string
}

export type Configuration = {
  prompts: PromptSlot[]
  models?: unknown[]
  tools?: unknown[]
  knowledge?: unknown[]
  evaluations?: unknown[]
  memory_policy?: unknown
  runtime_limits?: unknown
}

export type AgentVersion = {
  id: string
  agent_id: string
  version_number: number
  configuration: Configuration
  config_checksum: string
  created_at: string
}

export type AgentForm = {
  name: string
  description: string
  tags: string
  visibility: 'public' | 'private'
  prompt: string
}
