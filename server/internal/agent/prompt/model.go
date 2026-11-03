package prompt

// Slot 是一个可独立启停和排序的提示词片段。
type Slot struct {
	Name      string   `json:"name"`
	Content   string   `json:"content"`
	Enabled   bool     `json:"enabled"`
	Order     int      `json:"order"`
	Variables []string `json:"variables"`
	Scope     string   `json:"scope"`
}

// Snapshot 是保存有效配置时固化的提示词配置。
type Snapshot struct {
	Slots    []Slot `json:"slots"`
	Rendered string `json:"rendered"`
	Checksum string `json:"checksum"`
}
