package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

var _ tools.SpiritSynthesisPort = (*SpiritSynthesisService)(nil)

// synthesisEventPublisher adapts event.Bus to biz.SynthesisEventPublisher.
type synthesisEventPublisher struct {
	bus event.Bus
}

func (p *synthesisEventPublisher) PublishSynthesisCompleted(ctx context.Context, spiritSessionID string, output *biz.SynthesisOutput) {
	if p.bus == nil {
		return
	}
	type richResult struct {
		TeamID      string `json:"team_id"`
		TeamName    string `json:"team_name"`
		TaskName    string `json:"task_name"`
		Status      string `json:"status"`
		Summary     string `json:"summary"`
		KeyFindings string `json:"key_findings,omitempty"`
	}
	richResults := make([]richResult, 0, len(output.TeamResults))
	for _, r := range output.TeamResults {
		richResults = append(richResults, richResult{
			TeamID:      r.TeamID,
			TeamName:    r.TeamName,
			TaskName:    r.TaskName,
			Status:      r.Status,
			Summary:     r.Summary,
			KeyFindings: r.KeyFindings,
		})
	}
	env := event.NewEnvelope(event.EnvelopeTypeSpiritSynthesisCompleted, "spirit-synthesis", spiritSessionID)
	env.Metadata = map[string]interface{}{
		"spirit_session_id": spiritSessionID,
		"strategy":          string(output.Strategy),
		"team_count":        len(output.TeamResults),
		"content":           output.Content,
		"team_results":      richResults,
	}
	p.bus.Publish(ctx, env)
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
	eventBus event.Bus,
	lg loggateway.Logger,
) *SpiritSynthesisService {
	pub := &synthesisEventPublisher{bus: eventBus}
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
