package service

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/pkg/strutil"
)

// MemoryLLMExtractor implements biz.MemoryTextExtractor using OpenAI-compatible chat completions.
type MemoryLLMExtractor struct {
	Agents      *biz.AgentUsecase
	Sessions    *biz.SessionUsecase
	ModelCatalog *biz.LlmProviderModelUsecase
	HTTP        *http.Client
}

func NewMemoryLLMExtractor(
	agents *biz.AgentUsecase,
	sessions *biz.SessionUsecase,
	catalog *biz.LlmProviderModelUsecase,
	httpClient *http.Client,
) *MemoryLLMExtractor {
	return &MemoryLLMExtractor{
		Agents:      agents,
		Sessions:    sessions,
		ModelCatalog: catalog,
		HTTP:        httpClient,
	}
}

var _ biz.MemoryTextExtractor = (*MemoryLLMExtractor)(nil)

func (e *MemoryLLMExtractor) ExtractFacts(ctx context.Context, in biz.ConsolidateInput) ([]biz.MemoryProposal, error) {
	if e == nil || e.ModelCatalog == nil || e.HTTP == nil {
		return nil, biz.ErrLLMExtractorUnavailable
	}
	if strings.TrimSpace(os.Getenv("MEMORY_WORKER_LLM_DISABLED")) == "1" {
		return nil, biz.ErrLLMExtractorUnavailable
	}
	transcript := buildConsolidateTranscript(in.Messages)
	if strings.TrimSpace(transcript) == "" {
		return nil, nil
	}

	prov, mod, err := e.resolveProviderModel(ctx, in)
	if err != nil {
		return nil, err
	}
	if prov == "" || mod == "" {
		return nil, biz.ErrLLMExtractorUnavailable
	}

	row, err := e.ModelCatalog.GetByProviderAndModel(ctx, prov, mod)
	if err != nil {
		return nil, err
	}
	var cfg chatagent.ProviderAPIConfig
	chatagent.MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
	}

	msgs := []chatagent.OpenAICompatMessage{
		{Role: "system", Content: compress.MemoryExtractSystemPromptV2},
		{Role: "user", Content: "Conversation excerpt:\n\n" + transcript},
	}

	tools := []map[string]any{compress.ExtractMemoryFactsFunctionSchema}
	text, toolCalls, _, _, callErr := chatagent.CallOpenAICompatChatWithTools(callCtx, e.HTTP, cfg, mod, msgs, tools)

	var proposals []biz.MemoryProposal
	var extractionQuality float64

	if callErr == nil {
		for _, tc := range toolCalls {
			if tc.Name == compress.ExtractMemoryFactsFunctionName {
				facts, _, parseErr := compress.ParseMemoryExtractFunctionCallArgs(tc.Arguments)
				if parseErr == nil && len(facts) > 0 {
					extractionQuality = biz.ExtractionQualityFunctionCall
					proposals = convertFactsToProposals(facts, in.Messages, extractionQuality)
				}
				break
			}
		}
	}

	if len(proposals) == 0 && callErr == nil && strings.TrimSpace(text) != "" {
		facts, parseErr := compress.ParseMemoryExtractJSON(text)
		if parseErr == nil && len(facts) > 0 {
			extractionQuality = biz.ExtractionQualityJSONMode
			proposals = convertFactsToProposals(facts, in.Messages, extractionQuality)
		}
	}

	if len(proposals) == 0 && callErr != nil {
		return nil, callErr
	}

	return proposals, nil
}

func convertFactsToProposals(facts []compress.MemoryExtractFact, messages []biz.ConsolidateMessage, quality float64) []biz.MemoryProposal {
	out := make([]biz.MemoryProposal, 0, len(facts))
	seen := make(map[string]struct{}, len(facts))
	for _, f := range facts {
		key := strings.ToLower(f.Statement)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		p := biz.MemoryProposal{
			Layer:             biz.MemoryLayerL3,
			Statement:         f.Statement,
			Topics:            f.Topics,
			SourceMessageID:   biz.ResolveProposalMessageID(f.Statement, messages),
			ExtractionQuality: quality,
			IsPIISensitive:    f.IsPIISensitive,
		}
		if f.Confidence > 0 {
			p.Topics = append(p.Topics, f.SubjectType)
		}
		out = append(out, p)
	}
	return out
}

func (e *MemoryLLMExtractor) resolveProviderModel(ctx context.Context, in biz.ConsolidateInput) (prov, mod string, err error) {
	var ag biz.Agent
	if e.Agents != nil {
		agentID := strings.TrimSpace(in.AgentID)
		if agentID == "" {
			agentID = strings.TrimSpace(in.AppName)
		}
		if agentID != "" {
			ag, err = e.Agents.Get(ctx, agentID)
			if err != nil {
				return "", "", err
			}
		}
	}
	var sess biz.Session
	if e.Sessions != nil && strings.TrimSpace(in.SessionID) != "" {
		sess, err = e.Sessions.Get(ctx, in.SessionID)
		if err != nil {
			return "", "", err
		}
	}
	prov, mod = memoryWorkerProviderModel(sess, ag)
	return
}

func memoryWorkerProviderModel(sess biz.Session, ag biz.Agent) (prov, mod string) {
	if ag.Settings != nil {
		p := strings.TrimSpace(ag.Settings.MemoryWorkerProvider)
		m := strings.TrimSpace(ag.Settings.MemoryWorkerModel)
		if p != "" && m != "" {
			return p, m
		}
		p = strings.TrimSpace(ag.Settings.L0CompressProvider)
		m = strings.TrimSpace(ag.Settings.L0CompressModel)
		if p != "" && m != "" {
			return p, m
		}
	}
	return strutil.FirstNonEmpty(sess.DefaultProvider, ag.Provider), strutil.FirstNonEmpty(sess.DefaultModel, ag.Model)
}

func buildConsolidateTranscript(messages []biz.ConsolidateMessage) string {
	lines := make([]struct{ Role, Content string }, 0, len(messages))
	for _, m := range messages {
		lines = append(lines, struct{ Role, Content string }{Role: m.Role, Content: m.Content})
	}
	return compress.BuildMemoryExtractTranscript(lines)
}
