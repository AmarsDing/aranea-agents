package mcpmount

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/mcptoolset"
)

// AppendEffectiveMCPServerToolsets appends one ADK MCP toolset per effective server row.
// ctx is passed to stdio MCP subprocess creation (optional; see [TransportFromConfig]).
func AppendEffectiveMCPServerToolsets(ctx context.Context, out *[]tool.Toolset, servers []biz.EffectiveMCPServer) error {
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
		pc, err := parseServerConfigJSON(cfgJSON)
		if err != nil {
			return fmt.Errorf("mcp server %q: %w", key, err)
		}
		transport, err := TransportFromConfig(ctx, pc)
		if err != nil {
			return fmt.Errorf("mcp server %q: %w", key, err)
		}
		cfg := mcptoolset.Config{
			Transport:           transport,
			RequireConfirmation: pc.RequireUserCredentials,
		}
		base, err := mcptoolset.New(cfg)
		if err != nil {
			return fmt.Errorf("mcp server %q: %w", key, err)
		}
		if pred := toolPredicateForPrefix(pc.ToolPrefix); pred != nil {
			*out = append(*out, tool.FilterToolset(base, pred))
		} else {
			*out = append(*out, base)
		}
	}
	return nil
}
