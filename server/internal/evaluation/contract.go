package evaluation

// Result 保存评分结果和评分依据。
type Result struct {
	ID             string
	ConversationID string
	TurnID         string
	RubricID       string
	Score          float64
	Evidence       []Evidence
}

// Evidence 描述评分使用的可追溯证据。
type Evidence struct {
	Dimension string
	Excerpt   string
	Reason    string
}
