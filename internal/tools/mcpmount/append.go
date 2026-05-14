package mcpmount

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcmcp "trpc.group/trpc-go/trpc-agent-go/tool/mcp"
)

func AppendEffectiveMCPServerToolsets(ctx context.Context, out *[]trpctool.ToolSet, servers []biz.EffectiveMCPServer) error {
	if out == nil {
		return nil
	}
	for _, s := range servers {
		key := strings.TrimSpace(s.ServerKey)
		if key == "" {
			key = strings.TrimSpace(s.ID)
		}
		cfgJSON := strings.TrimSpace(s.ConfigJSON)
		if cfgJSON == "" {
			continue
		}
		sc, err := parseServerConfigJSON(cfgJSON)
		if err != nil {
			return fmt.Errorf("mcp server %q: %w", key, err)
		}
		connCfg := toTRPCConnectionConfig(sc)
		opts := []trpcmcp.ToolSetOption{trpcmcp.WithName(key)}
		if pred := toolFilterForPrefix(sc.ToolPrefix); pred != nil {
			opts = append(opts, trpcmcp.WithToolFilterFunc(pred))
		}
		ts := trpcmcp.NewMCPToolSet(connCfg, opts...)
		*out = append(*out, ts)
	}
	return nil
}

func toolFilterForPrefix(prefix string) trpctool.FilterFunc {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	return func(_ context.Context, t trpctool.Tool) bool {
		if t == nil || t.Declaration() == nil {
			return false
		}
		return strings.HasPrefix(t.Declaration().Name, prefix)
	}
}
