// Command openapi 导出由 Go 接口结构体生成的 OpenAPI，不依赖运行环境。
package main

import (
	"flag"
	"fmt"
	"os"

	"linq-bs/server/internal/bootstrap"
)

func main() {
	output := flag.String("output", "../docs/api/openapi.yaml", "OpenAPI 输出路径")
	flag.Parse()
	data, err := bootstrap.OpenAPI().YAML()
	if err == nil {
		err = os.WriteFile(*output, data, 0644)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
