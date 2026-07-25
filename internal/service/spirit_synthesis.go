package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

var _ tools.SpiritSynthesisPort = (*SpiritSynthesisService)(nil)

// synthesisCallTimeout caps the LLM call for spirit synthesis. C-24.
const synthesisCallTimeout = 90 * time.Second

// synthesisModelAdapter implements biz.SynthesisModelPort by resolving an LLM
// via the 2-tier strategy (system DefaultRefineLLM → first enabled catalog
// model) and delegating to biz.LLMCaller. C-24 fix: previously the synthesis
// engine only built a prompt string and never invoked any LLM.
type synthesisModelAdapter struct {
	llm     biz.LLMCaller
	sys     *biz.SystemSettingUsecase
	catalog *biz.LlmProviderModelUsecase
	lg      loggateway.Logger
}

// NewSynthesisModelAdapter constructs a biz.SynthesisModelPort backed by the
// dynamic LLM caller. Returns nil when llm is nil, signalling the engine to
// fall back to raw prompt output.
func NewSynthesisModelAdapter(
	llm biz.LLMCaller,
	sys *biz.SystemSettingUsecase,
	catalog *biz.LlmProviderModelUsecase,
	lg loggateway.Logger,
) biz.SynthesisModelPort {
	if llm == nil {
		return nil
	}
	return &synthesisModelAdapter{llm: llm, sys: sys, catalog: catalog, lg: lg}
}

func (a *synthesisModelAdapter) SynthesizeWithModel(ctx context.Context, system, user string) (string, error) {
	provider, model, err := a.resolveModel(ctx)
	if err != nil {
		return "", err
	}
	callCtx, cancel := context.WithTimeout(ctx, synthesisCallTimeout)
	defer cancel()
	text, _, err := a.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   system,
		User:     user,
	})
	if err != nil {
		return "", fmt.Errorf("synthesis llm call failed: %w", err)
	}
	return text, nil
}

// resolveModel picks the LLM config using a 2-tier fallback:
//  1. Platform DefaultRefineLLM system setting
//  2. First enabled model from catalog
func (a *synthesisModelAdapter) resolveModel(ctx context.Context) (string, string, error) {
	if a.sys != nil {
		s, err := a.sys.Get(ctx)
		if err == nil && strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
		if err != nil {
			a.lg.Warn("系统设置获取失败，尝试 catalog 回退",
				loggateway.StepID("spirit.synthesis.resolve_model"),
				loggateway.Err(err))
		}
	}
	if a.catalog != nil {
		models, err := a.catalog.List(ctx)
		if err == nil {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled {
					return m.Provider, m.Model, nil
				}
			}
		} else {
			a.lg.Warn("模型目录获取失败",
				loggateway.StepID("spirit.synthesis.resolve_model"),
				loggateway.Err(err))
		}
	}
	return "", "", apierror.Unavailable(apierror.DomainSpirit, "no LLM available for synthesis; configure DefaultRefineLLM in system settings")
}

// SpiritSynthesisService is a thin transport adapter that delegates all business
// logic to biz.SynthesisUsecase and translates domain errors to API errors.
// It backs the SynthesizeResults RPC and the synthesize_results tool; the
// user-facing summary report in chat is produced by the Spirit summary turn
// triggered by TeamStarter (no dedicated report step is published).
type SpiritSynthesisService struct {
	uc *biz.SynthesisUsecase
	lg loggateway.Logger
}

func NewSpiritSynthesisService(
	spiritUC *biz.SpiritTeamUsecase,
	engine *biz.SynthesisEngine,
	lg loggateway.Logger,
) *SpiritSynthesisService {
	uc := biz.NewSynthesisUsecase(spiritUC, engine, lg)
	return &SpiritSynthesisService{uc: uc, lg: lg}
}

func (s *SpiritSynthesisService) SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*biz.SynthesisOutput, error) {
	output, err := s.uc.SynthesizeResults(ctx, spiritSessionID, strategy)
	if err != nil {
		return nil, translateSynthesisError(err)
	}
	return output, nil
}

// translateSynthesisError maps biz-level domain errors to apierror for transport.
func translateSynthesisError(err error) error {
	switch err {
	case biz.ErrActiveTeamsExist:
		return apierror.BadRequest("SPIRIT", fmt.Sprintf("cannot synthesize: %s", err.Error()))
	case biz.ErrNoCompletedTeams:
		return apierror.BadRequest("SPIRIT", err.Error())
	case biz.ErrNoTeamResults:
		return apierror.BadRequest("SPIRIT", err.Error())
	case biz.ErrUnknownStrategy:
		return apierror.BadRequest("SPIRIT", err.Error())
	default:
		return err
	}
}
