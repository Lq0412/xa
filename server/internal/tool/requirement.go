package tool

// Requirement 描述工具运行时依赖的前置条件。
type Requirement string

const (
	RequireMemoryEnabled Requirement = "memory_enabled"
)

func requirementMet(req Requirement, memoryEnabled bool) bool {
	switch req {
	case RequireMemoryEnabled:
		return memoryEnabled
	default:
		return true
	}
}
