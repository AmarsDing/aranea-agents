package planner

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/a2ui"

	agent "trpc.group/trpc-go/trpc-agent-go/agent"
	model "trpc.group/trpc-go/trpc-agent-go/model"
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

func buildA2UI(pipeline *a2ui.Pipeline, plannerConfigJSON string) trpcplanner.Planner {
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
	inner := trpca2ui.New(opts...)
	if pipeline == nil {
		return inner
	}
	return &a2uiPlannerWrapper{
		Planner:  inner,
		pipeline: pipeline,
	}
}

// a2uiPlannerWrapper wraps the framework A2UI planner to integrate
// the local Pipeline for Surface management and event emission.
type a2uiPlannerWrapper struct {
	trpcplanner.Planner
	pipeline *a2ui.Pipeline
}

func (w *a2uiPlannerWrapper) ProcessPlanningResponse(
	ctx context.Context,
	invocation *agent.Invocation,
	response *model.Response,
) *model.Response {
	// Delegate to framework planner first
	result := w.Planner.ProcessPlanningResponse(ctx, invocation, response)
	if result == nil || w.pipeline == nil {
		return result
	}
	// Pipeline is available for future Plan→Graph conversion and event emission.
	// The actual Plan extraction and Graph execution will be triggered by
	// userAction events from the frontend, which flow through the normal
	// agent event pipeline.
	return result
}

func buildPlanner(kind, plannerConfigJSON string, pipeline *a2ui.Pipeline) trpcplanner.Planner {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "react":
		return trpcreact.New()
	case "a2ui":
		return buildA2UI(pipeline, plannerConfigJSON)
	case "builtin":
		return buildBuiltin(plannerConfigJSON)
	default:
		return nil
	}
}
