package service

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"
)

var _ tools.SpiritSynthesisPort = (*SpiritSynthesisService)(nil)

type SpiritSynthesisService struct {
	spiritUC *biz.SpiritTeamUsecase
	engine   *biz.SynthesisEngine
	eventBus contract.Bus
	lg       loggateway.Logger
}

func NewSpiritSynthesisService(
	spiritUC *biz.SpiritTeamUsecase,
	engine *biz.SynthesisEngine,
	eventBus contract.Bus,
	lg loggateway.Logger,
) *SpiritSynthesisService {
	return &SpiritSynthesisService{
		spiritUC: spiritUC,
		engine:   engine,
		eventBus: eventBus,
		lg:       lg,
	}
}

func (s *SpiritSynthesisService) SynthesizeResults(ctx context.Context, spiritSessionID string, strategy string) (*biz.SynthesisOutput, error) {
	teams, err := s.spiritUC.ListCompletedAndFailedTeams(ctx, spiritSessionID)
	if err != nil {
		return nil, err
	}
	teamResults := s.spiritUC.BuildCascadeBlockedResults(ctx, teams)
	if len(teamResults) == 0 {
		return nil, biz.ErrNoCompletedTeams
	}
	input := biz.SynthesisInput{
		TeamResults: teamResults,
		Strategy:    biz.SynthesisStrategy(strategy),
		SpiritQuery: s.spiritUC.GetSpiritQuery(ctx, spiritSessionID),
	}
	output, err := s.engine.Synthesize(ctx, input)
	if err != nil {
		return nil, err
	}
	if s.eventBus != nil {
		type slimResult struct {
			TeamID   string `json:"team_id"`
			TeamName string `json:"team_name"`
			Status   string `json:"status"`
		}
		slimResults := make([]slimResult, 0, len(teamResults))
		for _, r := range teamResults {
			slimResults = append(slimResults, slimResult{
				TeamID:   r.TeamID,
				TeamName: r.TeamName,
				Status:   r.Status,
			})
		}
		env := event.NewEnvelope(contract.EnvelopeTypeSpiritSynthesisCompleted, "spirit-synthesis", spiritSessionID)
		env.Metadata = map[string]interface{}{
			"spirit_session_id": spiritSessionID,
			"strategy":          string(output.Strategy),
			"team_count":        len(teamResults),
			"content":           output.Content,
			"team_results":      slimResults,
		}
		s.eventBus.Publish(ctx, env)
	}
	return output, nil
}
