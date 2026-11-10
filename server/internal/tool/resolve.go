package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"linq-bs/server/internal/agent/binding"
	"linq-bs/server/internal/agent/definition"
	"linq-bs/server/internal/framework/auth"
	"linq-bs/server/internal/infra/provider"
)

// Toolset 是一次 run 的工具快照。
type Toolset struct {
	Entries []Entry `json:"entries"`
	Hash    string  `json:"hash"`
}

// Entry 描述单个工具在快照中的生效配置。
type Entry struct {
	Spec     Spec            `json:"spec"`
	Enabled  bool            `json:"enabled"`
	Approval ApprovalMode    `json:"approval"`
	Timeout  time.Duration   `json:"timeout_ms"`
	MaxCalls int             `json:"max_calls"`
	Config   json.RawMessage `json:"configuration"`
	Reason   string          `json:"reason,omitempty"`
}

// ResolveInput 汇总绑定、记忆策略与客户端能力。
type ResolveInput struct {
	Bindings     []binding.ToolBinding
	MemoryPolicy definition.MemoryPolicy
	Principal    auth.Principal
	CanApprove   bool
}

// Resolve 根据 Catalog 与 Agent 绑定生成工具快照。
func (r *Registry) Resolve(in ResolveInput) (Toolset, error) {
	if r == nil {
		return Toolset{}, fmt.Errorf("注册表未初始化")
	}
	memoryEnabled := in.MemoryPolicy.Enabled
	entries := make([]Entry, 0, len(r.tools))
	for _, spec := range r.Catalog() {
		b, bound := bindingFor(spec.Name, in.Bindings)
		entry := Entry{Spec: spec}
		if bound {
			entry.Enabled = b.Enabled
			entry.Config = b.Configuration
			if b.Approval != "" {
				entry.Approval = ApprovalMode(b.Approval)
			} else {
				entry.Approval = spec.Approval.Default
			}
			if b.TimeoutMS > 0 {
				entry.Timeout = time.Duration(b.TimeoutMS) * time.Millisecond
			}
			if b.MaxCalls > 0 {
				entry.MaxCalls = b.MaxCalls
			}
		} else {
			entry.Enabled = spec.DefaultEnabled
			entry.Approval = spec.Approval.Default
		}
		if entry.Timeout == 0 {
			entry.Timeout = spec.DefaultTimeout
		}
		if entry.Timeout > 60*time.Second {
			entry.Timeout = 60 * time.Second
		}
		if entry.MaxCalls == 0 {
			entry.MaxCalls = spec.MaxCallsPerRun
		}
		if entry.MaxCalls > spec.MaxCallsPerRun {
			entry.MaxCalls = spec.MaxCallsPerRun
		}
		if !spec.Approval.Overridable && spec.Approval.Default == ApprovalAlways {
			entry.Approval = ApprovalAlways
		}
		if bound && len(b.AllowedRoles) > 0 && !rolesIntersect(b.AllowedRoles, in.Principal.Roles) {
			entry.Enabled = false
			entry.Reason = "role_denied"
		}
		if !spec.Available {
			entry.Enabled = false
			if entry.Reason == "" {
				entry.Reason = "placeholder"
			}
		}
		for _, req := range spec.Requires {
			if !requirementMet(req, memoryEnabled) {
				entry.Enabled = false
				if entry.Reason == "" {
					entry.Reason = "memory_disabled"
				}
			}
		}
		if !in.CanApprove && (entry.Approval == ApprovalAlways || spec.Approval.Default == ApprovalAlways) {
			entry.Enabled = false
			entry.Reason = "client_cannot_approve"
		}
		if !bound && !spec.DefaultEnabled {
			entry.Enabled = false
			if entry.Reason == "" {
				entry.Reason = "disabled"
			}
		}
		if bound && !b.Enabled {
			entry.Enabled = false
			if entry.Reason == "" {
				entry.Reason = "disabled"
			}
		}
		entries = append(entries, entry)
	}
	hash, err := toolsetHash(entries)
	if err != nil {
		return Toolset{}, err
	}
	return Toolset{Entries: entries, Hash: hash}, nil
}

func bindingFor(name string, bindings []binding.ToolBinding) (binding.ToolBinding, bool) {
	for _, b := range bindings {
		if b.Name == name {
			return b, true
		}
		if b.Name == "" && b.ToolVersionID != "" {
			continue
		}
	}
	return binding.ToolBinding{}, false
}

func rolesIntersect(allowed, actual []string) bool {
	for _, a := range allowed {
		if slices.Contains(actual, a) {
			return true
		}
	}
	return false
}

type hashEntry struct {
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Enabled   bool            `json:"enabled"`
	Approval  ApprovalMode    `json:"approval"`
	TimeoutMS int64           `json:"timeout_ms"`
	MaxCalls  int             `json:"max_calls"`
	Config    json.RawMessage `json:"configuration"`
	Reason    string          `json:"reason"`
}

func toolsetHash(entries []Entry) (string, error) {
	canonical := make([]hashEntry, 0, len(entries))
	for _, e := range entries {
		canonical = append(canonical, hashEntry{
			Name: e.Spec.Name, Version: e.Spec.Version, Enabled: e.Enabled, Approval: e.Approval,
			TimeoutMS: e.Timeout.Milliseconds(), MaxCalls: e.MaxCalls, Config: e.Config, Reason: e.Reason,
		})
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// ModelTools 返回发给模型的工具定义列表。
func (ts Toolset) ModelTools() []provider.ToolSpec {
	out := make([]provider.ToolSpec, 0, len(ts.Entries))
	for _, e := range ts.Entries {
		if !e.Enabled || !e.Spec.Available {
			continue
		}
		out = append(out, provider.ToolSpec{
			Name:        e.Spec.Name,
			Description: e.Spec.Description,
			Parameters:  e.Spec.InputSchema,
		})
	}
	return out
}

// EntryByName 查找快照中的条目。
func (ts Toolset) EntryByName(name string) (Entry, bool) {
	for _, e := range ts.Entries {
		if e.Spec.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}
