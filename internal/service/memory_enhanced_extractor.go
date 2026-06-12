package service

import (
	"context"
	"net/http"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
)

// MemoryEnhancedExtractor implements biz.EnhancedTextExtractor using OpenAI-compatible chat completions.
type MemoryEnhancedExtractor struct {
	agents       *biz.AgentUsecase
	sessions     *biz.SessionUsecase
	modelCatalog *biz.LlmProviderModelUsecase
	http         *http.Client
	llmDisabled  bool
}

// MemoryEnhancedExtractorConfig holds all dependencies for MemoryEnhancedExtractor.
type MemoryEnhancedExtractorConfig struct {
	Agents       *biz.AgentUsecase
	Sessions     *biz.SessionUsecase
	ModelCatalog *biz.LlmProviderModelUsecase
	HTTPClient   *http.Client
	LLMDisabled  bool
}

// NewMemoryEnhancedExtractor creates a new MemoryEnhancedExtractor.
func NewMemoryEnhancedExtractor(cfg MemoryEnhancedExtractorConfig) *MemoryEnhancedExtractor {
	return &MemoryEnhancedExtractor{
		agents:       cfg.Agents,
		sessions:     cfg.Sessions,
		modelCatalog: cfg.ModelCatalog,
		http:         cfg.HTTPClient,
		llmDisabled:  cfg.LLMDisabled,
	}
}

var _ biz.EnhancedTextExtractor = (*MemoryEnhancedExtractor)(nil)

func (e *MemoryEnhancedExtractor) ExtractEnhanced(ctx context.Context, in biz.ConsolidateInput) (*biz.EnhancedExtractionResult, error) {
	if e == nil || e.modelCatalog == nil || e.http == nil || e.llmDisabled {
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

	row, err := e.modelCatalog.GetByProviderAndModel(ctx, prov, mod)
	if err != nil {
		return nil, err
	}
	var cfg chatagent.ProviderAPIConfig
	chatagent.MergeProviderConfigJSON(row.ConfigJSON, &cfg)

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
	}

	msgs := []chatagent.OpenAICompatMessage{
		{Role: "system", Content: compress.EnhancedExtractSystemPrompt},
		{Role: "user", Content: "Conversation excerpt:\n\n" + transcript},
	}

	tools := []map[string]any{compress.EnhancedExtractFunctionSchema}
	text, toolCalls, _, _, callErr := chatagent.CallOpenAICompatChatWithTools(callCtx, e.http, cfg, mod, msgs, tools)

	var compressResult *compress.EnhancedExtractionResult

	if callErr == nil {
		for _, tc := range toolCalls {
			if tc.Name == compress.EnhancedExtractFunctionName {
				parsed, parseErr := compress.ParseEnhancedExtractionResult(tc.Arguments)
				if parseErr == nil && parsed != nil {
					compressResult = parsed
				}
				break
			}
		}
	}

	// Fallback: try parsing raw text as JSON.
	if compressResult == nil && callErr == nil && strings.TrimSpace(text) != "" {
		parsed, parseErr := compress.ParseEnhancedExtractionResult(text)
		if parseErr == nil && parsed != nil {
			compressResult = parsed
		}
	}

	if compressResult == nil && callErr != nil {
		return nil, callErr
	}

	if compressResult == nil {
		return nil, nil
	}

	return compressToBizResult(compressResult), nil
}

func (e *MemoryEnhancedExtractor) resolveProviderModel(ctx context.Context, in biz.ConsolidateInput) (prov, mod string, err error) {
	var ag biz.Agent
	if e.agents != nil {
		agentID := strings.TrimSpace(in.AgentID)
		if agentID == "" {
			agentID = strings.TrimSpace(in.AppName)
		}
		if agentID != "" {
			ag, err = e.agents.Get(ctx, agentID)
			if err != nil {
				return "", "", err
			}
		}
	}
	var sess biz.Session
	if e.sessions != nil && strings.TrimSpace(in.SessionID) != "" {
		sess, err = e.sessions.Get(ctx, in.SessionID)
		if err != nil {
			return "", "", err
		}
	}
	prov, mod = memoryWorkerProviderModel(sess, ag)
	return
}

// compressToBizResult converts compress types to biz types.
func compressToBizResult(r *compress.EnhancedExtractionResult) *biz.EnhancedExtractionResult {
	if r == nil {
		return nil
	}
	result := &biz.EnhancedExtractionResult{
		Episode: biz.EnhancedEpisodeData{
			Title:          r.Episode.Title,
			Goal:           r.Episode.Goal,
			Outcome:        r.Episode.Outcome,
			OutcomeSummary: r.Episode.OutcomeSummary,
			KeyDecisions:   r.Episode.KeyDecisions,
			KeyArtifacts:   r.Episode.KeyArtifacts,
			Importance:     r.Episode.Importance,
			Confidence:     r.Episode.Confidence,
		},
		Entities:  make([]biz.ExtractedEntity, 0, len(r.Entities)),
		Relations: make([]biz.ExtractedRelation, 0, len(r.Relations)),
	}
	for _, ent := range r.Entities {
		result.Entities = append(result.Entities, biz.ExtractedEntity{
			Name:        ent.Name,
			EntityType:  ent.EntityType,
			Description: ent.Description,
			Confidence:  ent.Confidence,
		})
	}
	for _, rel := range r.Relations {
		result.Relations = append(result.Relations, biz.ExtractedRelation{
			SourceEntity: rel.SourceEntity,
			TargetEntity: rel.TargetEntity,
			RelationType: rel.RelationType,
			Confidence:   rel.Confidence,
		})
	}
	return result
}
