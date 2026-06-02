package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var _ tools.SpiritSynthesisPort = (*SpiritSynthesisService)(nil)

type SpiritSynthesisService struct {
	spiritUC *biz.SpiritTeamUsecase
	engine   *biz.SynthesisEngine
	eventBus event.Bus
	lg       loggateway.Logger
}

func NewSpiritSynthesisService(
	spiritUC *biz.SpiritTeamUsecase,
	engine *biz.SynthesisEngine,
	eventBus event.Bus,
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
	activeTeams, activeErr := s.spiritUC.ListActiveTeams(ctx, spiritSessionID)
	if activeErr != nil {
		s.lg.Warn("查询活跃团队失败，跳过活跃检查",
			loggateway.StepID("spirit.synthesis.active_check_err"),
			loggateway.Err(activeErr),
		)
	} else if len(activeTeams) > 0 {
		return nil, kerrors.BadRequest("SPIRIT",
			fmt.Sprintf("cannot synthesize: %d team(s) still running/active, wait for completion", len(activeTeams)))
	}

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
		type richResult struct {
			TeamID      string `json:"team_id"`
			TeamName    string `json:"team_name"`
			TaskName    string `json:"task_name"`
			Status      string `json:"status"`
			Summary     string `json:"summary"`
			KeyFindings string `json:"key_findings,omitempty"`
		}
		richResults := make([]richResult, 0, len(teamResults))
		for _, r := range teamResults {
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
			"team_count":        len(teamResults),
			"content":           output.Content,
			"team_results":      richResults,
		}
		s.eventBus.Publish(ctx, env)
	}
	return output, nil
}
