package planner

import (
	"strings"

	trpcplanner "trpc.group/trpc-go/trpc-agent-go/planner"
	trpca2ui "trpc.group/trpc-go/trpc-agent-go/planner/a2ui"
	trpcbuiltin "trpc.group/trpc-go/trpc-agent-go/planner/builtin"
	trpcreact "trpc.group/trpc-go/trpc-agent-go/planner/react"
)

func buildBuiltin(plannerConfigJSON string) trpcplanner.Planner {
	cfg := parseBuiltinConfigJSON(plannerConfigJSON)
	opts := trpcbuiltin.Options{
		ReasoningEffort: cfg.ReasoningEffort,
		ThinkingEnabled: cfg.ThinkingEnabled,
		ThinkingTokens:  cfg.ThinkingTokens,
	}
	return trpcbuiltin.New(opts)
}

func buildA2UI(plannerConfigJSON string) trpcplanner.Planner {
	cfg := parseA2UIConfigJSON(plannerConfigJSON)
	var opts []trpca2ui.Option
	if s := strings.TrimSpace(cfg.Instruction); s != "" {
		opts = append(opts, trpca2ui.WithInstruction(s))
	}
	if s := strings.TrimSpace(cfg.ServerToClientWithStandardCatalogJSON); s != "" {
		opts = append(opts, trpca2ui.WithServerToClientWithStandardCatalogSchema(s))
	}
	if s := strings.TrimSpace(cfg.ClientToServerSchemaJSON); s != "" {
		opts = append(opts, trpca2ui.WithClientToServerSchema(s))
	}
	if s := strings.TrimSpace(cfg.ClientCapabilitiesSchemaJSON); s != "" {
		opts = append(opts, trpca2ui.WithClientCapabilitiesSchema(s))
	}
	if s := strings.TrimSpace(cfg.ServerToClientOnlySchemaJSON); s != "" {
		opts = append(opts, trpca2ui.WithServerToClientSchema(s))
	}
	if s := strings.TrimSpace(cfg.StandardCatalogDefinitionJSON); s != "" {
		opts = append(opts, trpca2ui.WithStandardCatalogDefinition(s))
	}
	if s := strings.TrimSpace(cfg.CatalogDescriptionSchemaJSON); s != "" {
		opts = append(opts, trpca2ui.WithCatalogDescriptionSchema(s))
	}
	if len(opts) == 0 {
		return trpca2ui.New()
	}
	return trpca2ui.New(opts...)
}

func buildPlanner(kind, plannerConfigJSON string) trpcplanner.Planner {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "react":
		return trpcreact.New()
	case "a2ui":
		return buildA2UI(plannerConfigJSON)
	case "builtin":
		return buildBuiltin(plannerConfigJSON)
	default:
		return nil
	}
}
