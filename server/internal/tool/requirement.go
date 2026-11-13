package tool

// Requirement 描述工具运行时依赖的前置条件。
type Requirement string

const (
	RequireMemoryEnabled  Requirement = "memory_enabled"
	RequireKnowledgeBound Requirement = "knowledge_bound"
)

func requirementMet(req Requirement, memoryEnabled, knowledgeBound bool) bool {
	switch req {
	case RequireMemoryEnabled:
		return memoryEnabled
	case RequireKnowledgeBound:
		return knowledgeBound
	default:
		return true
	}
}
