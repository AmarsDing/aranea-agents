package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// MemoryFactWriteAdjudicator implements biz.FactWriteAdjudicator: the LLM
// operation-semantics verdict (ADD/UPDATE/DELETE/NOOP) for contested fact
// candidates in the unified write pipeline (P1-3).
//
// Model resolution mirrors MemoryLLMExtractor: agent MemoryWorker →
// L0Compress → agent/session default via ModelCatalog.
type MemoryFactWriteAdjudicator struct {
	agents       *biz.AgentUsecase
	sessions     *biz.SessionUsecase
	modelCatalog *biz.LlmProviderModelUsecase
	rt           *provider.RoundTrip
	llmDisabled  bool
	lg           loggateway.Logger
}

// MemoryFactWriteAdjudicatorConfig holds all dependencies.
type MemoryFactWriteAdjudicatorConfig struct {
	Agents       *biz.AgentUsecase
	Sessions     *biz.SessionUsecase
	ModelCatalog *biz.LlmProviderModelUsecase
	RoundTrip    *provider.RoundTrip
	LLMDisabled  bool
	Logger       loggateway.Logger
}

// NewMemoryFactWriteAdjudicator creates the adjudicator. Returns nil when
// LLM access is impossible (pipeline then falls back to heuristic ADD).
func NewMemoryFactWriteAdjudicator(cfg MemoryFactWriteAdjudicatorConfig) *MemoryFactWriteAdjudicator {
	if cfg.ModelCatalog == nil || cfg.RoundTrip == nil || cfg.LLMDisabled {
		return nil
	}
	lg := cfg.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemoryFactWriteAdjudicator{
		agents:       cfg.Agents,
		sessions:     cfg.Sessions,
		modelCatalog: cfg.ModelCatalog,
		rt:           cfg.RoundTrip,
		llmDisabled:  cfg.LLMDisabled,
		lg:           lg.With(loggateway.Domain("fact_write_adjudicator")),
	}
}

var _ biz.FactWriteAdjudicator = (*MemoryFactWriteAdjudicator)(nil)

// AdjudicateFactWrites verdicts each contested candidate in one batch LLM
// call. Unknown candidates simply get no verdict (pipeline falls back to
// heuristic for them).
func (a *MemoryFactWriteAdjudicator) AdjudicateFactWrites(ctx context.Context, agentID, userID string, items []biz.FactAdjudicationItem) ([]biz.FactAdjudicationVerdict, error) {
	if a == nil || len(items) == 0 {
		return nil, nil
	}

	prov, mod := a.resolveProviderModel(ctx, agentID)
	if prov == "" || mod == "" {
		return nil, biz.ErrLLMExtractorUnavailable
	}
	m, err := provider.TRPCModelForProviderModel(ctx, a.modelCatalog, a.rt, prov, mod, a.lg)
	if err != nil {
		return nil, err
	}

	promptItems := make([]compress.FactAdjudicationPromptItem, 0, len(items))
	for _, it := range items {
		neighbors := make([]compress.FactAdjudicationPromptNeighbor, 0, len(it.Neighbors))
		for _, n := range it.Neighbors {
			neighbors = append(neighbors, compress.FactAdjudicationPromptNeighbor{
				ID:        n.FactID,
				Statement: n.Statement,
				Kind:      n.FactKind,
			})
		}
		promptItems = append(promptItems, compress.FactAdjudicationPromptItem{
			Statement: it.Candidate.Statement,
			Kind:      it.Candidate.FactKind,
			Neighbors: neighbors,
		})
	}

	callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req := trpcmodel.NewRequest([]trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: compress.FactAdjudicationSystemPrompt},
		{Role: trpcmodel.RoleUser, Content: compress.BuildFactAdjudicationPrompt(promptItems)},
	})
	respCh, err := m.GenerateContent(callCtx, req)
	if err != nil {
		return nil, err
	}
	var text string
	for resp := range respCh {
		if resp.Error != nil {
			return nil, resp.Error
		}
		for _, c := range resp.Choices {
			if c.Delta.Content != "" {
				text += c.Delta.Content
			}
			if c.Message.Content != "" {
				text += c.Message.Content
			}
		}
	}

	rawVerdicts, err := compress.ParseFactAdjudicationResponse(text)
	if err != nil {
		return nil, err
	}
	out := make([]biz.FactAdjudicationVerdict, 0, len(rawVerdicts))
	for _, v := range rawVerdicts {
		out = append(out, biz.FactAdjudicationVerdict{
			Statement:    v.Statement,
			Operation:    biz.FactWriteOperation(v.Operation),
			TargetFactID: v.TargetID,
		})
	}
	return out, nil
}

// resolveProviderModel resolves the adjudication model from agent settings
// (MemoryWorker → L0Compress → agent default). Session defaults apply when a
// session is reachable via the agent's recent context; userID is currently
// informational only.
func (a *MemoryFactWriteAdjudicator) resolveProviderModel(ctx context.Context, agentID string) (prov, mod string) {
	var ag biz.Agent
	if a.agents != nil && strings.TrimSpace(agentID) != "" {
		var err error
		ag, err = a.agents.Get(ctx, agentID)
		if err != nil {
			a.lg.Warn("adjudicator: agent lookup failed, no model", loggateway.Err(err))
			return "", ""
		}
	}
	p, m := memoryWorkerProviderModel(biz.Session{}, ag)
	return p, m
}
