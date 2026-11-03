package bootstrap

import "context"

// Server 负责组装管理接口、用户接口和 Agent 运行依赖。
type Server struct{}

// NewServer 创建 server 进程的依赖装配入口。
func NewServer() *Server {
	return &Server{}
}

// Run 启动 server 进程。
func (s *Server) Run(ctx context.Context) error {
	panic("TODO: implement server bootstrap")
}
