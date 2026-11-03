package prompt

// Slot 是一个可独立启停和排序的提示词片段。
type Slot struct {
	Name      string
	Content   string
	Enabled   bool
	Order     int
	Variables []string
	Scope     string
}

// Snapshot 是保存有效配置时固化的提示词配置。
type Snapshot struct {
	Slots    []Slot
	Rendered string
	Checksum string
}
