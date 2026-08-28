package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type turnCuePrefetchKey struct{}

// TurnCuePrefetch holds memory/knowledge cues built during BUILD so the
// first BeforeModel hook can skip the DB+embed round trip.
type TurnCuePrefetch struct {
	knowledge *prefetchedKnowledgeCue
	memory    *prefetchedMemoryCue
}

type prefetchedKnowledgeCue struct {
	query string
	cue   string
	cited []biz.KnowledgeChunk
}

type prefetchedMemoryCue struct {
	keyword         string
	profileCue      string
	recallCue       string
	recallHits      []biz.CompositeRecallHit
	injectedFactIDs []string
}

// WithTurnCuePrefetch attaches BUILD-time cue prefetch to ctx.
func WithTurnCuePrefetch(ctx context.Context, p *TurnCuePrefetch) context.Context {
	if p == nil || ctx == nil {
		return ctx
	}
	if p.knowledge == nil && p.memory == nil {
		return ctx
	}
	return context.WithValue(ctx, turnCuePrefetchKey{}, p)
}

func turnCuePrefetchFromContext(ctx context.Context) *TurnCuePrefetch {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(turnCuePrefetchKey{}).(*TurnCuePrefetch)
	return p
}

// PrefetchTurnCues runs the expensive knowledge ListCollections+search and
// memory profile/composite recall without an invocation. Failures are
// non-fatal: the BeforeModel hooks rebuild on cache miss.
func PrefetchTurnCues(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, userContent, sessionID, userID string) *TurnCuePrefetch {
	out := &TurnCuePrefetch{}
	query := strings.TrimSpace(userContent)
	if query != "" {
		query = cleanRecallQuery(query)
	}
	toolsEnabled := ag.Settings != nil && ag.Settings.ToolsEnabled
	groundedOnly := biz.ParseAgentKnowledgeConfig(ag.ConfigJSON).GroundedOnly
	if deps.KnowledgeUsecase != nil {
		lg := deps.Logger()
		if lg == nil {
			lg = loggateway.NewNoop()
		}
		cue, cited := buildKnowledgeCue(ctx, deps.KnowledgeUsecase, lg, query, toolsEnabled, groundedOnly, knowledgetool.MemoryL3GroundedFromContext(ctx))
		if cue != "" {
			out.knowledge = &prefetchedKnowledgeCue{query: query, cue: cue, cited: cited}
		}
	}

	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	biz.ClampSpecialistL3Scopes(&policy, ag)
	if !policy.MasterEnabled {
		return out
	}
	keyword := RecallKeywordFromMessages([]trpcmodel.Message{trpcmodel.NewUserMessage(userContent)})
	mem := &prefetchedMemoryCue{keyword: keyword}
	rt := biz.MemoryRuntimeContext{
		AgentID: strings.TrimSpace(ag.ID),
		UserID:  strings.TrimSpace(userID),
	}
	if ag.Settings != nil {
		rt.Workspace = strings.TrimSpace(ag.Settings.Workspace)
	}
	if policy.InjectL3 {
		summary, fallbackIDs, _ := MemorySummaryCue(ctx, deps.MemoryProfileCardReader, deps.MemoryPreferenceLister, rt.AgentID, rt.UserID)
		mem.profileCue = summary
		mem.injectedFactIDs = append(mem.injectedFactIDs, fallbackIDs...)
	}
	if policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil {
		if composite, hits := CompositeMemoryCueWithHits(ctx, deps.MemoryCompositeRecall, ag, policy, rt, sessionID, keyword, 0, ProactiveHitsFromContext(ctx)); composite != "" {
			mem.recallCue = composite
			mem.recallHits = hits
			for _, h := range hits {
				if h.Layer == "L3" && strings.TrimSpace(h.FactID) != "" {
					mem.injectedFactIDs = append(mem.injectedFactIDs, strings.TrimSpace(h.FactID))
				}
			}
		}
	}
	if mem.profileCue != "" || mem.recallCue != "" {
		out.memory = mem
	}
	return out
}
