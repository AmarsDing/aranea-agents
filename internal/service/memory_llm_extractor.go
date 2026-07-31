package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// MemoryLLMExtractor implements biz.MemoryTextExtractor using provider-routed LLM calls.
type MemoryLLMExtractor struct {
	agents       *biz.AgentUsecase
	sessions     *biz.SessionUsecase
	modelCatalog *biz.LlmProviderModelUsecase
	rt           *provider.RoundTrip
	llmDisabled  bool
	lg           loggateway.Logger
}

// MemoryLLMExtractorConfig holds all dependencies for MemoryLLMExtractor.
type MemoryLLMExtractorConfig struct {
	Agents       *biz.AgentUsecase
	Sessions     *biz.SessionUsecase
	ModelCatalog *biz.LlmProviderModelUsecase
	RoundTrip    *provider.RoundTrip
	LLMDisabled  bool
	Logger       loggateway.Logger
}

func NewMemoryLLMExtractor(cfg MemoryLLMExtractorConfig) *MemoryLLMExtractor {
	return &MemoryLLMExtractor{
		agents:       cfg.Agents,
		sessions:     cfg.Sessions,
		modelCatalog: cfg.ModelCatalog,
		rt:           cfg.RoundTrip,
		llmDisabled:  cfg.LLMDisabled,
		lg:           cfg.Logger,
	}
}

var _ biz.MemoryTextExtractor = (*MemoryLLMExtractor)(nil)

func (e *MemoryLLMExtractor) ExtractFacts(ctx context.Context, in biz.ConsolidateInput) ([]biz.MemoryProposal, error) {
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
		callCtx, cancel = context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
	}

	toolDecls := mapSchemaToToolDecls([]map[string]any{compress.ExtractMemoryFactsFunctionSchema})

	req := &trpcmodel.Request{
		Messages: []trpcmodel.Message{
			trpcmodel.NewSystemMessage(compress.MemoryExtractSystemPromptV2),
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

	var proposals []biz.MemoryProposal
	var extractionQuality float64
	var jsonParseErr error

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

	if len(proposals) == 0 && strings.TrimSpace(text) != "" {
		facts, parseErr := compress.ParseMemoryExtractJSON(text)
		if parseErr == nil && len(facts) > 0 {
			extractionQuality = biz.ExtractionQualityJSONMode
			proposals = convertFactsToProposals(facts, in.Messages, extractionQuality)
		} else if parseErr != nil {
			jsonParseErr = parseErr
		}
	}

	if len(proposals) == 0 && jsonParseErr != nil {
		return nil, biz.ErrLLMExtractionFailed
	}

	return proposals, nil
}

type toolCallResult struct {
	Name      string
	Arguments string
}

// staticToolDecl wraps a trpctool.Declaration to implement the Tool interface.
type staticToolDecl struct {
	decl *trpctool.Declaration
}

func (s *staticToolDecl) Declaration() *trpctool.Declaration { return s.decl }

// mapSchemaToToolDecls converts map[string]any tool schemas to trpcmodel-compatible tool declarations.
func mapSchemaToToolDecls(tools []map[string]any) map[string]trpctool.Tool {
	toolDecls := make(map[string]trpctool.Tool, len(tools))
	for _, t := range tools {
		name, _ := t["name"].(string)
		desc, _ := t["description"].(string)
		params, _ := t["parameters"].(map[string]any)
		decl := &trpctool.Declaration{
			Name:        name,
			Description: desc,
		}
		if params != nil {
			schemaBytes, _ := json.Marshal(params)
			var schema trpctool.Schema
			if json.Unmarshal(schemaBytes, &schema) == nil {
				decl.InputSchema = &schema
			}
		}
		toolDecls[name] = &staticToolDecl{decl: decl}
	}
	return toolDecls
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
			SubjectType:       f.SubjectType,
			Scope:             f.Scope,
			Confidence:        f.Confidence,
		}
		out = append(out, p)
	}
	return out
}

func (e *MemoryLLMExtractor) resolveProviderModel(ctx context.Context, in biz.ConsolidateInput) (prov, mod string, err error) {
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
