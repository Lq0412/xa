package tool

// Definition 描述工具的稳定身份。
type Definition struct {
	ID          string
	Name        string
	Description string
}

// Version 描述一个可绑定到 Agent 版本的工具实现。
type Version struct {
	ID           string
	DefinitionID string
	Version      string
	Schema       []byte
}
