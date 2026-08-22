package agent

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

const intentToolHintLimit = 8

// newIntentToolHintPromoteBeforeHook pre-activates deferred tools named in
// the intent artifact (tool_hints) or ranked from refined_goal, so the main
// model sees their schema on the first call without a tool_load round-trip.
func newIntentToolHintPromoteBeforeHook(dm *deferred.DeferredToolManager, lg loggateway.Logger) callbacks.Callback {
	if dm == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return callbacks.NewBeforeAgentHook(2, func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
		catalog, _ := dm.CatalogSnapshot()
		if len(catalog) == 0 {
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		}
		art := intentArtifactFromInvocation(ctx, args)
		goal := ""
		var llmHints []string
		if art != nil {
			goal = strings.TrimSpace(art.RefinedGoal)
			if len(art.SearchHints) > 0 {
				goal = strings.TrimSpace(goal + " " + strings.Join(art.SearchHints, " "))
			}
			llmHints = art.ToolHints
		}
		if goal == "" && len(llmHints) == 0 {
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		}
		hints := deferred.ResolveToolHints(catalog, goal, llmHints, intentToolHintLimit)
		activated := 0
		for _, name := range hints {
			resolved, ok := dm.ResolveName(name)
			if !ok {
				resolved = name
			}
			if !dm.IsInCatalog(resolved) {
				continue
			}
			if dm.IsActivated(ctx, resolved) {
				activated++
				continue
			}
			if _, err := dm.Activate(ctx, resolved); err != nil {
				lg.Warn("intent tool hint activate failed",
					loggateway.StepID("agent.intent.tool_hint"),
					loggateway.Str("tool", resolved),
					loggateway.Err(err),
				)
				metrics.DeferredToolActivationTotal.WithLabelValues(resolved, "hint_error").Inc()
				continue
			}
			metrics.DeferredToolActivationTotal.WithLabelValues(resolved, "hint").Inc()
			activated++
		}
		if activated > 0 {
			lg.Info("intent tool hints pre-activated",
				loggateway.StepID("agent.intent.tool_hint"),
				loggateway.Int("activated", activated),
			)
		}
		return &trpcagent.BeforeAgentResult{Context: ctx}, nil
	})
}

func intentArtifactFromInvocation(ctx context.Context, args *trpcagent.BeforeAgentArgs) *intent.Artifact {
	if args != nil && args.Invocation != nil {
		if art := parseIntentArtifactFromUserText(args.Invocation.Message.Content); art != nil {
			return art
		}
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil {
		return nil
	}
	if art := parseIntentArtifactFromUserText(inv.Message.Content); art != nil {
		return art
	}
	if inv.Session == nil {
		return nil
	}
	raw, ok := inv.Session.GetState("intent_artifact")
	if !ok || len(raw) == 0 {
		return nil
	}
	var art intent.Artifact
	if json.Unmarshal(raw, &art) != nil || strings.TrimSpace(art.RefinedGoal) == "" {
		return nil
	}
	return &art
}

func parseIntentArtifactFromUserText(text string) *intent.Artifact {
	const marker = "Derived intent (align your plan and tools to this JSON):"
	idx := strings.Index(text, marker)
	if idx < 0 {
		return nil
	}
	art, _ := parseArtifactJSONForHints(text[idx+len(marker):])
	return art
}

func parseArtifactJSONForHints(text string) (*intent.Artifact, string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, ""
	}
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			text = text[i : j+1]
		}
	}
	var art intent.Artifact
	if json.Unmarshal([]byte(text), &art) != nil {
		return nil, ""
	}
	if strings.TrimSpace(art.RefinedGoal) == "" && len(art.ToolHints) == 0 {
		return nil, ""
	}
	return &art, text
}
