package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// memoryInjectMarker is a hidden prefix injected into MemoryInject system
// messages so they can be reliably identified during context-compaction
// rebuild.  It is an XML comment so LLMs treat it as inert metadata.
const memoryInjectMarker = "<!-- aranea:memory_inject -->\n"

// isMemoryInjectMessage reports whether msg was injected by the
// MemoryInject before-model hook (identified by the hidden marker prefix).
func isMemoryInjectMessage(msg trpcmodel.Message) bool {
	return msg.Role == trpcmodel.RoleSystem &&
		strings.HasPrefix(msg.Content, memoryInjectMarker)
}

// memoryInjectCueContent wraps a cue string with the identification marker.
func memoryInjectCueContent(cue string) string {
	return memoryInjectMarker + cue
}

// memoryInjectStripMarker removes the identification marker from a cue string.
func memoryInjectStripMarker(content string) string {
	return strings.TrimPrefix(content, memoryInjectMarker)
}

func memoryRuntimeContext(inv *trpcagent.Invocation, ag biz.Agent) biz.MemoryRuntimeContext {
	rt := biz.MemoryRuntimeContext{
		AgentID: strings.TrimSpace(ag.ID),
	}
	if inv != nil && inv.Session != nil {
		rt.UserID = strings.TrimSpace(inv.Session.UserID)
		rt.Workspace = sessionStateString(inv.Session.State, "workspace")
		rt.TeamID = sessionStateString(inv.Session.State, "team_id")
	}
	if rt.Workspace == "" && ag.Settings != nil {
		rt.Workspace = strings.TrimSpace(ag.Settings.Workspace)
	}
	return rt
}

func sessionStateString(state map[string][]byte, key string) string {
	if state == nil {
		return ""
	}
	if b, ok := state[key]; ok {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// MemoryCueResult holds the structured output of memory cue building.
type MemoryCueResult struct {
	// L1Cue is the L1 session summary cue (injectable, changes after compression).
	L1Cue string
	// RecallCue is the combined L2/L3/L4 recall cue (keyword-based, changes every turn).
	RecallCue string
}

// IsEmpty reports whether the result contains any cue content.
func (r *MemoryCueResult) IsEmpty() bool {
	return r.L1Cue == "" && r.RecallCue == ""
}

// JoinCues returns the combined cue text for injection.
func (r *MemoryCueResult) JoinCues() string {
	var parts []string
	if r.L1Cue != "" {
		parts = append(parts, r.L1Cue)
	}
	if r.RecallCue != "" {
		parts = append(parts, r.RecallCue)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func newMemoryInjectBeforeHook(ag biz.Agent, deps TRPCBuilderDeps) callbacks.Callback {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.AnyInject() {
		return nil
	}
	hasDep := (policy.InjectL1 || policy.InjectL4) && deps.MemoryAdmin != nil
	hasDep = hasDep || (policy.RecallL2 && deps.MemoryL2Recall != nil)
	hasDep = hasDep || (policy.InjectL3 && deps.MemoryL3Recall != nil)
	hasDep = hasDep || (policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil)
	if !hasDep {
		return nil
	}
	return callbacks.NewBeforeModelHook(5, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		result := buildRuntimeMemoryCue(ctx, deps, ag, args.Request.Messages)
		if result.IsEmpty() {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		cue := result.JoinCues()
		sys := trpcmodel.NewSystemMessage(memoryInjectCueContent(cue))
		args.Request.Messages = append([]trpcmodel.Message{sys}, args.Request.Messages...)
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func buildRuntimeMemoryCue(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, messages []trpcmodel.Message) *MemoryCueResult {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled {
		return &MemoryCueResult{}
	}
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return &MemoryCueResult{}
	}
	rt := memoryRuntimeContext(inv, ag)
	sessionID := strings.TrimSpace(inv.Session.ID)
	keyword := RecallKeywordFromMessages(messages)

	result := &MemoryCueResult{}

	// L1: session summary (changes after compression rebuild)
	if policy.InjectL1 {
		if l1 := L1MemoryCue(ctx, deps.MemoryAdmin, ag, policy, sessionID); l1 != "" {
			result.L1Cue = l1
		}
	}

	// L2/L3/L4: recall-based cues (keyword-driven, changes every turn)
	var recallParts []string
	if policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil {
		if composite := CompositeMemoryCue(ctx, deps.MemoryCompositeRecall, ag, policy, rt, sessionID, keyword, 0); composite != "" {
			recallParts = append(recallParts, composite)
		}
	} else {
		if policy.RecallL2 {
			if l2 := L2MemoryCue(ctx, deps.MemoryL2Recall, ag, policy, sessionID, keyword, 0); l2 != "" {
				recallParts = append(recallParts, l2)
			}
		}
		if policy.InjectL3 {
			if l3 := L3MemoryCue(ctx, deps.MemoryL3Recall, ag, policy, rt, keyword, 0); l3 != "" {
				recallParts = append(recallParts, l3)
			}
		}
	}
	if policy.InjectL4 {
		if l4 := L4MemoryCue(ctx, deps.MemoryAdmin, ag, policy, keyword); l4 != "" {
			recallParts = append(recallParts, l4)
		}
	}
	if len(recallParts) > 0 {
		result.RecallCue = strings.TrimSpace(strings.Join(recallParts, "\n\n"))
	}

	return result
}

func lastUserMessageText(messages []trpcmodel.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != trpcmodel.RoleUser {
			continue
		}
		if t := strings.TrimSpace(messages[i].Content); t != "" {
			if len(t) > 120 {
				return safeTruncate(t, 120)
			}
			return t
		}
	}
	return ""
}

// RebuildMemoryInjectForCompaction re-executes the MemoryInject hook after
// context-compaction rebuild. It updates the L1 cue in the system messages
// without affecting other hooks' cache breakpoint positions.
//
// When L0 context compaction fires, the framework rebuilds the request from
// the session summary + tail events. The L1 session summary may have been
// updated by the compression itself, but the old MemoryInject TextBlock still
// contains the pre-compaction L1 cue. This function re-runs only the L1 cue
// resolution and patches the existing MemoryInject system message in-place.
func RebuildMemoryInjectForCompaction(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, req *trpcmodel.Request) {
	if req == nil {
		return
	}
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.InjectL1 {
		return
	}
	result := buildRuntimeMemoryCue(ctx, deps, ag, req.Messages)
	if result.L1Cue == "" {
		return
	}

	// Find and replace the existing MemoryInject system message in-place
	// to preserve other hooks' cache breakpoint positions.
	newCue := memoryInjectCueContent(result.JoinCues())
	for i, msg := range req.Messages {
		if isMemoryInjectMessage(msg) {
			req.Messages[i] = trpcmodel.NewSystemMessage(newCue)
			return
		}
	}

	// If no existing MemoryInject message found (edge case: first turn after
	// compaction with no prior inject), prepend a new one.
	sys := trpcmodel.NewSystemMessage(newCue)
	req.Messages = append([]trpcmodel.Message{sys}, req.Messages...)
}
