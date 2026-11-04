import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import { ApiError, api } from './api'
import type { Agent, AgentForm, Configuration, User } from './types'

const TOKEN_KEY = 'xa.web.session'

const emptyForm: AgentForm = {
  name: '',
  description: '',
  tags: '',
  visibility: 'private',
  prompt: '',
}

const emptyConfiguration: Configuration = { prompts: [] }

// App 负责认证状态和管理页切换，业务请求集中在 api 模块。
export default function App() {
  const [token, setToken] = useState(() => window.localStorage.getItem(TOKEN_KEY) || '')
  const [user, setUser] = useState<User | null>(null)
  const [booting, setBooting] = useState(Boolean(token))

  const signOut = useCallback(async () => {
    if (token) {
      await api.logout(token).catch(() => undefined)
    }
    window.localStorage.removeItem(TOKEN_KEY)
    setToken('')
    setUser(null)
  }, [token])

  useEffect(() => {
    if (!token) {
      setBooting(false)
      return
    }
    api.me(token)
      .then(setUser)
      .catch(() => signOut())
      .finally(() => setBooting(false))
  }, [signOut, token])

  const handleLogin = (nextToken: string, nextUser: User) => {
    window.localStorage.setItem(TOKEN_KEY, nextToken)
    setToken(nextToken)
    setUser(nextUser)
  }

  if (booting) {
    return <div className="center-page"><div className="loading-card">正在恢复登录状态…</div></div>
  }
  if (!token || !user) {
    return <AuthPage onLogin={handleLogin} />
  }
  return <Dashboard token={token} user={user} onSignOut={signOut} />
}

function AuthPage({ onLogin }: { onLogin: (token: string, user: User) => void }) {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      if (mode === 'register') {
        await api.register({ email, password, display_name: displayName })
      }
      const result = await api.login({ email, password })
      onLogin(result.token, result.user)
    } catch (cause) {
      setError(readError(cause))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="center-page auth-shell">
      <div className="auth-intro">
        <span className="eyebrow">XA / AGENT WORKSPACE</span>
        <h1>把 Agent 配置成<br /><em>可以直接使用的能力。</em></h1>
        <p>先从账号和 Agent 管理开始。保存后的配置会立即生成版本，后续对话会固定使用对应版本。</p>
      </div>
      <form className="panel auth-card" onSubmit={submit}>
        <div className="form-heading">
          <div>
            <span className="eyebrow">{mode === 'login' ? 'WELCOME BACK' : 'GET STARTED'}</span>
            <h2>{mode === 'login' ? '登录工作台' : '创建账号'}</h2>
          </div>
          <span className="status-dot" />
        </div>
        {mode === 'register' && (
          <label>显示名称<input value={displayName} onChange={(event) => setDisplayName(event.target.value)} placeholder="例如：Lina" /></label>
        )}
        <label>邮箱<input type="email" required value={email} onChange={(event) => setEmail(event.target.value)} placeholder="you@example.com" autoComplete="email" /></label>
        <label>密码<input type="password" required minLength={8} value={password} onChange={(event) => setPassword(event.target.value)} placeholder="至少 8 个字符" autoComplete={mode === 'login' ? 'current-password' : 'new-password'} /></label>
        {error && <div className="alert error">{error}</div>}
        <button className="primary-button" disabled={submitting}>{submitting ? '处理中…' : mode === 'login' ? '登录' : '注册并登录'}</button>
        <button type="button" className="text-button" onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError('') }}>
          {mode === 'login' ? '还没有账号？创建一个' : '已有账号？返回登录'}
        </button>
      </form>
    </div>
  )
}

function Dashboard({ token, user, onSignOut }: { token: string; user: User; onSignOut: () => Promise<void> }) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [form, setForm] = useState<AgentForm>(emptyForm)
  const [configuration, setConfiguration] = useState<Configuration>(emptyConfiguration)
  const [versionId, setVersionId] = useState('')
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  const selected = useMemo(() => agents.find((agent) => agent.id === selectedId) || null, [agents, selectedId])
  const isAdmin = user.role === 'admin'

  const loadAgents = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const result = await api.listAgents(token)
      setAgents(result.agents)
    } catch (cause) {
      setError(readError(cause))
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => { void loadAgents() }, [loadAgents])

  const startCreate = () => {
    setSelectedId(null)
    setForm({ ...emptyForm, visibility: isAdmin ? 'public' : 'private' })
    setConfiguration(emptyConfiguration)
    setVersionId('')
    setMessage('正在创建新的 Agent')
    setError('')
  }

  const selectAgent = async (agent: Agent) => {
    setSelectedId(agent.id)
    setError('')
    setMessage('正在读取配置…')
    try {
      const version = await api.getAgentConfig(token, agent.id)
      const prompts = version.configuration.prompts || []
      setForm({
        name: agent.name,
        description: agent.description,
        tags: (agent.tags || []).join(', '),
        visibility: agent.visibility,
        prompt: prompts.find((prompt) => prompt.enabled)?.content || prompts[0]?.content || '',
      })
      setConfiguration(version.configuration)
      setVersionId(version.id)
      setMessage(`当前版本 v${version.version_number}`)
    } catch (cause) {
      setError(readError(cause))
      setMessage('')
    }
  }

  const save = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSaving(true)
    setError('')
    setMessage('正在保存…')
    const nextConfiguration: Configuration = {
      ...configuration,
      prompts: form.prompt.trim() ? [{ name: 'system', content: form.prompt.trim(), enabled: true, order: 0, variables: [], scope: 'agent' }] : [],
    }
    try {
      const result = selected
        ? await api.saveAgent(token, selected.id, versionId, form, nextConfiguration)
        : await api.createAgent(token, form, nextConfiguration)
      await loadAgents()
      setSelectedId(result.agent.id)
      setVersionId(result.current_version_id)
      setConfiguration(nextConfiguration)
      setMessage('已保存，新配置会用于新会话')
    } catch (cause) {
      setError(readError(cause))
      setMessage('')
    } finally {
      setSaving(false)
    }
  }

  const setStatus = async (enabled: boolean) => {
    if (!selected) return
    setError('')
    try {
      await api.setAgentStatus(token, selected.id, enabled)
      await loadAgents()
      setMessage(enabled ? 'Agent 已启用' : 'Agent 已停用')
    } catch (cause) {
      setError(readError(cause))
    }
  }

  const remove = async () => {
    if (!selected || !window.confirm(`确定删除“${selected.name}”吗？历史会话会保留为只读。`)) return
    setError('')
    try {
      await api.deleteAgent(token, selected.id)
      await loadAgents()
      startCreate()
      setMessage('Agent 已删除')
    } catch (cause) {
      setError(readError(cause))
    }
  }

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand"><span className="brand-mark">x</span><span>xa workspace</span></div>
        <div className="account"><span className="account-name">{user.display_name || user.email}</span><span className="role-chip">{isAdmin ? '管理员' : '用户'}</span><button className="text-button" onClick={() => void onSignOut()}>退出</button></div>
      </header>
      <main className="workspace">
        <aside className="sidebar">
          <div className="sidebar-heading"><div><span className="eyebrow">CONFIGURATION</span><h2>我的 Agents</h2></div><button className="icon-button" onClick={startCreate} aria-label="新建 Agent">+</button></div>
          <p className="muted">公共 Agent 和你创建的私有 Agent 会显示在这里。</p>
          <div className="agent-list">
            {loading && <div className="empty-state">正在读取…</div>}
            {!loading && agents.length === 0 && <div className="empty-state"><strong>还没有 Agent</strong><span>创建第一个配置开始使用。</span></div>}
            {agents.map((agent) => (
              <button key={agent.id} className={`agent-row ${selectedId === agent.id ? 'selected' : ''}`} onClick={() => void selectAgent(agent)}>
                <span className={`agent-avatar ${agent.visibility}`}>{agent.name.slice(0, 1).toUpperCase()}</span>
                <span className="agent-summary"><strong>{agent.name}</strong><small>{agent.visibility === 'public' ? '公共' : '私有'} · {agent.status === 'active' ? '运行中' : '已停用'}</small></span>
                <span className={`status-pill ${agent.status}`}>{agent.status === 'active' ? '●' : '○'}</span>
              </button>
            ))}
          </div>
        </aside>
        <section className="editor-area">
          <div className="editor-heading"><div><span className="eyebrow">AGENT EDITOR</span><h1>{selected ? selected.name : '新建 Agent'}</h1></div><span className="live-label"><span className="status-dot" />保存即生效</span></div>
          <form className="panel editor-card" onSubmit={save}>
            <div className="section-heading"><div><h3>基本信息</h3><p>这些内容会显示在 Agent 列表和选择页面。</p></div>{selected && <span className={`large-status ${selected.status}`}>{selected.status === 'active' ? '运行中' : '已停用'}</span>}</div>
            <div className="form-grid">
              <label>名称<input required maxLength={120} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="例如：英语面试教练" /></label>
              <label>可见范围<select value={form.visibility} disabled={!isAdmin} onChange={(event) => setForm({ ...form, visibility: event.target.value as AgentForm['visibility'] })}><option value="private">私有 Agent</option><option value="public">公共 Agent</option></select>{!isAdmin && <small className="field-note">普通用户只能创建私有 Agent</small>}</label>
            </div>
            <label>描述<textarea rows={3} maxLength={2000} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="说明这个 Agent 适合做什么" /></label>
            <label>标签<span className="label-hint">用逗号分隔</span><input value={form.tags} onChange={(event) => setForm({ ...form, tags: event.target.value })} placeholder="英语, 面试, 口语" /></label>
            <div className="section-heading prompt-heading"><div><h3>系统提示词</h3><p>先配置 Agent 的核心行为，模型和工具绑定会在后续阶段接入。</p></div><span className="config-state">{form.prompt.length} 字符</span></div>
            <textarea className="prompt-editor" rows={11} value={form.prompt} onChange={(event) => setForm({ ...form, prompt: event.target.value })} placeholder="你是一名耐心的英语教练…" />
            {message && <div className="alert success">{message}</div>}
            {error && <div className="alert error">{error}</div>}
            <div className="editor-actions"><button className="primary-button" disabled={saving}>{saving ? '保存中…' : '保存配置'}</button>{selected && <><button type="button" className="secondary-button" onClick={() => void setStatus(selected.status !== 'active')}>{selected.status === 'active' ? '停用 Agent' : '重新启用'}</button><button type="button" className="danger-button" onClick={() => void remove()}>删除</button></>}</div>
          </form>
        </section>
      </main>
    </div>
  )
}

function readError(cause: unknown) {
  if (cause instanceof ApiError) {
    return cause.requestId ? `${cause.message}（请求编号：${cause.requestId}）` : cause.message
  }
  if (cause instanceof Error) return cause.message
  return '请求失败，请稍后重试'
}
