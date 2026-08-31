package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	"aranea-agents/pkg/loggateway"

	"golang.org/x/sync/errgroup"
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

// PrefetchTurnCues runs knowledge ListCollections+search and memory
// profile/composite recall in parallel so a 2s knowledge miss cannot starve
// the 4s BUILD prefetch budget. Failures are non-fatal: BeforeModel hooks
// rebuild on cache miss.
func PrefetchTurnCues(ctx context.Context, deps TRPCBuilderDeps, ag biz.Agent, userContent, sessionID, userID string) *TurnCuePrefetch {
	out := &TurnCuePrefetch{}
	query := strings.TrimSpace(userContent)
	if query != "" {
		query = cleanRecallQuery(query)
	}
	toolsEnabled := ag.Settings != nil && ag.Settings.ToolsEnabled
	kbTools := agentHasKnowledgeSearch(ag)
	groundedOnly := biz.ParseAgentKnowledgeConfig(ag.ConfigJSON).GroundedOnly
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	biz.ClampSpecialistL3Scopes(&policy, ag)

	eg, egCtx := errgroup.WithContext(ctx)
	if deps.KnowledgeUsecase != nil {
		eg.Go(func() error {
			lg := deps.Logger()
			if lg == nil {
				lg = loggateway.NewNoop()
			}
			cue, cited := buildKnowledgeCue(egCtx, deps.KnowledgeUsecase, lg, query, toolsEnabled, kbTools, groundedOnly, knowledgetool.MemoryL3GroundedFromContext(egCtx))
			if cue != "" {
				out.knowledge = &prefetchedKnowledgeCue{query: query, cue: cue, cited: cited}
			}
			return nil
		})
	}
	// P6-N7：直复闲聊轮跳过记忆预取（与注入侧 SkipForDirectReply 闸同口径）。
	if policy.MasterEnabled && !intent.SkipForDirectReply(userContent) {
		eg.Go(func() error {
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
				summary, fallbackIDs, _ := MemorySummaryCue(egCtx, deps.MemoryProfileCardReader, deps.MemoryPreferenceLister, rt.AgentID, rt.UserID)
				mem.profileCue = summary
				mem.injectedFactIDs = append(mem.injectedFactIDs, fallbackIDs...)
			}
			if policy.RecallL2 && policy.InjectL3 && deps.MemoryCompositeRecall != nil {
				if composite, hits := CompositeMemoryCueWithHits(egCtx, deps.MemoryCompositeRecall, ag, policy, rt, sessionID, keyword, 0, ProactiveHitsFromContext(egCtx)); composite != "" {
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
			return nil
		})
	}
	_ = eg.Wait()
	return out
}
