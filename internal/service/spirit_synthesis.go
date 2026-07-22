package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
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

// synthesisEventPublisher adapts the v2 event pipeline to
// biz.SynthesisEventPublisher (B.10.17): the execution report is published as
// a persistent StepCreatedEvent (Kind=notice) whose Content carries the JSON
// envelope, so the report survives page refresh. seq is preferred
// (persist + WS); eventBus is the v1-only fallback (WS only).
type synthesisEventPublisher struct {
	seq          rt.EventPublisher
	bus          biz.EventBus
	taskV2Reader biz.TaskV2Reader
	lg           loggateway.Logger
}

// executionReportEnvelope is the Step.Content JSON contract (B.10.17.4).
// Field set mirrors biz.SynthesisOutput plus version/kind discriminator.
type executionReportEnvelope struct {
	Version       int                       `json:"version"`
	Kind          string                    `json:"kind"`
	Content       string                    `json:"content"`
	Strategy      biz.SynthesisStrategy     `json:"strategy"`
	Degraded      bool                      `json:"degraded,omitempty"`
	Overview      *biz.ExecutionOverview    `json:"overview,omitempty"`
	TeamResults   []biz.TeamSynthesisResult `json:"team_results"`
	Deliverables  []biz.DeliverableItem     `json:"deliverables,omitempty"`
	SynthesizedAt string                    `json:"synthesized_at"`
}

// executionReportEnvelopeVersion is the envelope schema version.
const executionReportEnvelopeVersion = 1

// executionReportEnvelopeKind discriminates the notice payload for the
// frontend NoticeBlock branch.
const executionReportEnvelopeKind = "execution_report"

func (p *synthesisEventPublisher) PublishSynthesisCompleted(ctx context.Context, spiritSessionID string, output *biz.SynthesisOutput) {
	if p.seq == nil && p.bus == nil {
		return
	}
	if output == nil {
		return
	}
	envelope := executionReportEnvelope{
		Version:       executionReportEnvelopeVersion,
		Kind:          executionReportEnvelopeKind,
		Content:       output.Content,
		Strategy:      output.Strategy,
		Degraded:      output.Degraded,
		Overview:      output.Overview,
		TeamResults:   output.TeamResults,
		Deliverables:  output.Deliverables,
		SynthesizedAt: output.SynthesizedAt,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		p.lg.Warn("执行报告信封序列化失败，跳过发布",
			loggateway.StepID("spirit.synthesis.envelope_marshal_err"),
			loggateway.Err(err),
		)
		return
	}
	now := time.Now()
	step := biz.Step{
		ID:              uuid.NewString(),
		SessionID:       spiritSessionID,
		SpiritSessionID: spiritSessionID,
		TaskID:          resolveLatestUserTaskID(ctx, p.taskV2Reader, p.lg, spiritSessionID),
		Kind:            biz.StepKindNotice,
		NoticeType:      "synthesis_completed",
		Content:         string(raw),
		Status:          biz.StepStatusCompleted,
		StartedAt:       now,
		CompletedAt:     &now,
		Version:         1,
		AuthorAgentKey:  "spirit-synthesis",
	}
	ev := biz.NewStepCreatedEvent(step)
	if p.seq != nil {
		p.seq.Publish(ctx, ev)
		return
	}
	p.bus.Publish(ctx, ev)
}

// SpiritSynthesisService is a thin transport adapter that delegates all business
// logic to biz.SynthesisUsecase and translates domain errors to API errors.
type SpiritSynthesisService struct {
	uc *biz.SynthesisUsecase
	lg loggateway.Logger
}

func NewSpiritSynthesisService(
	spiritUC *biz.SpiritTeamUsecase,
	engine *biz.SynthesisEngine,
	eventBus biz.EventBus,
	seq rt.EventPublisher,
	taskV2Reader biz.TaskV2Reader,
	lg loggateway.Logger,
) *SpiritSynthesisService {
	pub := &synthesisEventPublisher{seq: seq, bus: eventBus, taskV2Reader: taskV2Reader, lg: lg}
	uc := biz.NewSynthesisUsecase(spiritUC, engine, pub, lg)
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
