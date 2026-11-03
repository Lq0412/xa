package rubric

// Rubric 是可绑定到 Agent 版本的评价规则。
type Rubric struct {
	ID          string
	WorkspaceID string
	Name        string
	Dimensions  []Dimension
	Status      string
}

// Dimension 是评分维度及其权重。
type Dimension struct {
	Name   string
	Weight float64
	Rule   string
}
