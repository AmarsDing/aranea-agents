package agent

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"os"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/decide"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// mcpShadowEmittedKey marks one invocation so tool-call loops do not
// re-list MCP servers and re-emit the same choice.
const mcpShadowEmittedKey = "aranea.mcp_shadow_emitted"

// newMCPChoiceShadowBeforeHook records the incumbent MCP server set for
// JEV Phase-1 mcp_server_choice. The hook is a no-op unless
// ARANEA_JEV_SHADOW=on. It never changes the request. ConfigJSON is not
// copied into the shadow state (secrets stay on the assembly path).
func newMCPChoiceShadowBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	if deps.MCPTooling == nil || deps.DecisionCollector == nil {
		return nil
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return nil
	}
	return callbacks.NewBeforeModelHook(4, callbacks.LayerSemiStatic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if os.Getenv("ARANEA_JEV_SHADOW") != "on" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
			if _, done := inv.GetState(mcpShadowEmittedKey); done {
				return &trpcmodel.BeforeModelResult{Context: ctx}, nil
			}
		}
		task := lastUserMessageText(args.Request.Messages)
		if task == "" {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		servers, err := deps.MCPTooling.EffectiveServersForAgent(ctx, agentID)
		if err != nil {
			deps.Logger().Warn("mcp shadow list failed",
				loggateway.StepID("agent.mcp_shadow"),
				loggateway.Err(err))
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		emitMCPServerChoiceShadow(ctx, decide.NewRecorder(deps.DecisionCollector, deps.Logger()), task, servers)
		if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
			inv.SetState(mcpShadowEmittedKey, true)
		}
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

// emitMCPServerChoiceShadow writes one incumbent record.
// Zero servers → key none (filter excluded everything).
// One server → that server key (the only legal mount).
// Several servers → key mount_all: the live path mounts the whole filtered
// set and does not pick one server. Names live in state.mcp_servers and
// Incumbent.Scores; ConfigJSON is never copied.
func emitMCPServerChoiceShadow(ctx context.Context, rec *decide.Recorder, task string, servers []biz.EffectiveMCPServer) {
	if rec == nil || !rec.Enabled() || strings.TrimSpace(task) == "" {
		return
	}
	names := make([]string, 0, len(servers))
	scores := make(map[string]float64, len(servers))
	for _, s := range servers {
		name := strings.TrimSpace(s.ServerKey)
		if name == "" {
			name = strings.TrimSpace(s.ID)
		}
		if name == "" {
			continue
		}
		names = append(names, name)
		scores[name] = 1
	}
	key := "none"
	method := "filter"
	matched := false
	switch len(names) {
	case 0:
	case 1:
		key = names[0]
		method = "filter_mount_all"
		matched = true
	default:
		key = "mount_all"
		method = "filter_mount_all"
		matched = true
	}
	if len("q=mcp_server_choice;k="+key) > 64 {
		key = "mount_all"
	}
	rec.Emit(ctx, decide.ShadowEvent{
		CorrelationID: mcpShadowCorrID(task, key),
		QuestionID:    "mcp_server_choice",
		State: map[string]any{
			"task":        task,
			"mcp_servers": names,
		},
		Incumbent: decide.Incumbent{
			Key:     key,
			Scores:  scores,
			Method:  method,
			Matched: matched,
		},
	})
}

func mcpShadowCorrID(task, key string) string {
	h := sha1.New()
	h.Write([]byte("mcp_server_choice"))
	h.Write([]byte{0})
	h.Write([]byte(task))
	h.Write([]byte{0})
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
