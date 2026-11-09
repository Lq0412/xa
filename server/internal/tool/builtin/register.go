package builtin

import (
	"fmt"

	"linq-bs/server/internal/tool"
)

// RegisterProduction 向注册表装配生产环境内置工具。
func RegisterProduction(r *tool.Registry, deps Deps) error {
	if r == nil {
		return fmt.Errorf("注册表不能为空")
	}
	tools := []tool.Tool{
		&CurrentTime{},
		NewMemorySearch(deps.Memory),
		NewMemoryWrite(deps.Memory),
		NewMemoryForget(deps.Memory),
		NewToolResultRead(deps.DB),
		&KbSearch{},
		&AskAgent{},
	}
	for _, t := range tools {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}
