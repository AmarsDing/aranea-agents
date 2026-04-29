package server

import (
	"time"

	"aranea-agents/internal/conf"

	"github.com/go-kratos/kratos/v2/transport"
	sse "github.com/tx7do/kratos-transport/transport/sse"
)

var (
	_ transport.Server = (*sse.Server)(nil)
)

// NewSSEServer 使用 tx7do/kratos-transport/transport/sse，**独立监听**（与 Kratos HTTP 不同端口）。
// 配置来自 internal/conf/conf.proto 中 Server.sse，经 Kratos config Scan 注入。
func NewSSEServer(c *conf.Server) *sse.Server {
	if c == nil || c.GetSse() == nil || !c.GetSse().GetEnable() {
		return nil
	}
	s := c.GetSse()
	network := s.GetNetwork()
	if network == "" {
		network = "tcp"
	}
	addr := s.GetAddr()
	if addr == "" {
		addr = ":8001"
	}
	path := s.GetPath()
	if path == "" {
		path = "/"
	}
	timeout := 120 * time.Second
	if s.GetTimeout() != nil {
		timeout = s.GetTimeout().AsDuration()
	}
	return sse.NewServer(
		sse.WithNetwork(network),
		sse.WithAddress(addr),
		sse.WithPath(path),
		sse.WithCodec("json"),
		sse.WithTimeout(timeout),
		sse.WithAutoStream(true),
	)
}
