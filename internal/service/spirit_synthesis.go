package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

var _ tools.SpiritSynthesisPort = (*SpiritSynthesisService)(nil)

// synthesisEventPublisher adapts biz.ActivityEventBus to biz.SynthesisEventPublisher.
type synthesisEventPublisher struct {
	bus biz.ActivityEventBus
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
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventCompleted,
		Activity: biz.Activity{
			ID:              uuid.NewString(),
			Kind:            biz.ActivityKindSession,
			Status:          biz.ActivityStatusCompleted,
			Timestamp:       time.Now().UTC(),
			SpiritSessionID: spiritSessionID,
			AgentKey:        "spirit-synthesis",
			Stage:           "synthesis_completed",
			Meta: map[string]any{
				"spirit_session_id": spiritSessionID,
				"strategy":          string(output.Strategy),
				"team_count":        len(output.TeamResults),
				"content":           output.Content,
				"team_results":      richResults,
			},
		},
		Domain: biz.ActivityDomainChat,
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
	activityBus biz.ActivityEventBus,
	lg loggateway.Logger,
) *SpiritSynthesisService {
	pub := &synthesisEventPublisher{bus: activityBus}
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
