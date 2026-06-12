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

// MemoryEnhancedExtractor implements biz.EnhancedTextExtractor using provider-routed LLM calls.
type MemoryEnhancedExtractor struct {
	agents       *biz.AgentUsecase
	sessions     *biz.SessionUsecase
	modelCatalog *biz.LlmProviderModelUsecase
	rt           *provider.RoundTrip
	llmDisabled  bool
	lg           loggateway.Logger
}

// MemoryEnhancedExtractorConfig holds all dependencies for MemoryEnhancedExtractor.
type MemoryEnhancedExtractorConfig struct {
	Agents       *biz.AgentUsecase
	Sessions     *biz.SessionUsecase
	ModelCatalog *biz.LlmProviderModelUsecase
	RoundTrip    *provider.RoundTrip
	LLMDisabled  bool
	Logger       loggateway.Logger
}

// NewMemoryEnhancedExtractor creates a new MemoryEnhancedExtractor.
func NewMemoryEnhancedExtractor(cfg MemoryEnhancedExtractorConfig) *MemoryEnhancedExtractor {
	return &MemoryEnhancedExtractor{
		agents:       cfg.Agents,
		sessions:     cfg.Sessions,
		modelCatalog: cfg.ModelCatalog,
		rt:           cfg.RoundTrip,
		llmDisabled:  cfg.LLMDisabled,
		lg:           cfg.Logger,
	}
}

var _ biz.EnhancedTextExtractor = (*MemoryEnhancedExtractor)(nil)

func (e *MemoryEnhancedExtractor) ExtractEnhanced(ctx context.Context, in biz.ConsolidateInput) (*biz.EnhancedExtractionResult, error) {
	if e == nil || e.modelCatalog == nil || e.rt == nil || e.llmDisabled {
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

	m, err := provider.TRPCModelForProviderModel(ctx, e.modelCatalog, e.rt, prov, mod, e.lg)
	if err != nil {
		return nil, err
	}

	callCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
	}

	toolDecls := mapSchemaToToolDecls([]map[string]any{compress.EnhancedExtractFunctionSchema})

	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(compress.EnhancedExtractSystemPrompt),
			trpcmodel.NewUserMessage("Conversation excerpt:\n\n" + transcript),
		},
		Tools: toolDecls,
	}

	respCh, err := m.GenerateContent(callCtx, req)
	if err != nil {
		return nil, err
	}

	var text string
	var toolCalls []toolCallResult
	for resp := range respCh {
		if resp.Error != nil {
			return nil, biz.ErrLLMExtractionFailed
		}
		for _, c := range resp.Choices {
			if c.Delta.Content != "" {
				text += c.Delta.Content
			}
			if c.Message.Content != "" {
				text += c.Message.Content
			}
			for _, tc := range c.Message.ToolCalls {
				toolCalls = append(toolCalls, toolCallResult{
					Name:      tc.Function.Name,
					Arguments: string(tc.Function.Arguments),
				})
			}
		}
	}
	text = strings.TrimSpace(text)

	var compressResult *compress.EnhancedExtractionResult

	for _, tc := range toolCalls {
		if tc.Name == compress.EnhancedExtractFunctionName {
			parsed, parseErr := compress.ParseEnhancedExtractionResult(tc.Arguments)
			if parseErr == nil && parsed != nil {
				compressResult = parsed
			}
			break
		}
	}

	// Fallback: try parsing raw text as JSON.
	if compressResult == nil && strings.TrimSpace(text) != "" {
		parsed, parseErr := compress.ParseEnhancedExtractionResult(text)
		if parseErr == nil && parsed != nil {
			compressResult = parsed
		}
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
