import type { Agent, AgentForm, AgentVersion, Configuration, User } from './types'

type ApiErrorBody = {
  code?: string
  message?: string
  request_id?: string
}

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId: string

  constructor(status: number, body: ApiErrorBody) {
    super(body.message || `请求失败（${status}）`)
    this.name = 'ApiError'
    this.status = status
    this.code = body.code || 'request_failed'
    this.requestId = body.request_id || ''
  }
}

type RequestOptions = RequestInit & { token?: string }

// 所有页面请求都经过这里，统一处理 Bearer 令牌和服务端错误结构。
async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (options.token) {
    headers.set('Authorization', `Bearer ${options.token}`)
  }

  const response = await fetch(path, { ...options, headers })
  if (response.status === 204) {
    return undefined as T
  }
  const body = (await response.json()) as T | ApiErrorBody
  if (!response.ok) {
    throw new ApiError(response.status, body as ApiErrorBody)
  }
  return body as T
}

export type LoginResult = { token: string; user: User }

export const api = {
  register(input: { email: string; password: string; display_name: string }) {
    return request<User>('/api/v1/auth/register', { method: 'POST', body: JSON.stringify(input) })
  },

  login(input: { email: string; password: string }) {
    return request<LoginResult>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify(input) })
  },

  me(token: string) {
    return request<User>('/api/v1/auth/me', { token })
  },

  logout(token: string) {
    return request<void>('/api/v1/auth/logout', { method: 'POST', token })
  },

  listAgents(token: string) {
    return request<{ agents: Agent[] }>('/api/v1/agents', { token })
  },

  getAgent(token: string, agentId: string) {
    return request<Agent>(`/api/v1/agents/${encodeURIComponent(agentId)}`, { token })
  },

  getAgentConfig(token: string, agentId: string) {
    return request<AgentVersion>(`/api/v1/agents/${encodeURIComponent(agentId)}/config`, { token })
  },

  createAgent(token: string, form: AgentForm, configuration: Configuration) {
    return request<{ agent: Agent; current_version_id: string }>('/api/v1/agents', {
      method: 'POST',
      token,
      body: JSON.stringify(agentPayload(form, configuration)),
    })
  },

  saveAgent(token: string, agentId: string, expectedVersionId: string, form: AgentForm, configuration: Configuration) {
    return request<{ agent: Agent; current_version_id: string }>(`/api/v1/agents/${encodeURIComponent(agentId)}/config`, {
      method: 'PUT',
      token,
      body: JSON.stringify({ ...agentPayload(form, configuration), expected_version_id: expectedVersionId }),
    })
  },

  setAgentStatus(token: string, agentId: string, enabled: boolean) {
    const action = enabled ? 'enable' : 'offline'
    return request<void>(`/api/v1/agents/${encodeURIComponent(agentId)}/${action}`, { method: 'POST', token })
  },

  deleteAgent(token: string, agentId: string) {
    return request<void>(`/api/v1/agents/${encodeURIComponent(agentId)}`, { method: 'DELETE', token })
  },
}

function agentPayload(form: AgentForm, configuration: Configuration) {
  return {
    name: form.name.trim(),
    description: form.description.trim(),
    tags: form.tags.split(',').map((tag) => tag.trim()).filter(Boolean),
    visibility: form.visibility,
    configuration,
  }
}
