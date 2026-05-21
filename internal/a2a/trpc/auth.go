package trpc

import (
	"aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"

	a2aclient "trpc.group/trpc-go/trpc-a2a-go/client"
)

// a2aProxyClientAuthOptions maps catalog auth_type/auth_config_json to A2A client options.
func a2aProxyClientAuthOptions(cfg biz.A2AProxyConfig) ([]a2aclient.Option, error) {
	return a2a.ProxyClientAuthOptions(cfg)
}
