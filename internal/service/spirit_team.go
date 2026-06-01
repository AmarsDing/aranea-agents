package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type SpiritTeamAssembler struct {
	spiritUC *biz.SpiritTeamUsecase
	bus      event.Bus
	lg       loggateway.Logger
}

func NewSpiritTeamAssembler(
	spiritUC *biz.SpiritTeamUsecase,
	bus event.Bus,
	lg loggateway.Logger,
) *SpiritTeamAssembler {
	return &SpiritTeamAssembler{
		spiritUC: spiritUC,
		bus:      bus,
		lg:       lg,
	}
}

func (a *SpiritTeamAssembler) AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, error) {
	spiritSessionID := strings.TrimSpace(params.SpiritSessionID)

	a.lg.Info("精灵团队组装开始",
		loggateway.StepID("spirit.team.assemble"),
		loggateway.Str("spirit_session_id", spiritSessionID),
		loggateway.Str("mode", params.Mode),
		loggateway.Int("agent_count", len(params.AgentIDs)),
	)

	result, err := a.spiritUC.AssembleTeam(ctx, params)
	if err != nil {
		a.lg.Error("精灵团队组装失败",
			loggateway.StepID("spirit.team.assemble_fail"),
			loggateway.Str("spirit_session_id", spiritSessionID),
			loggateway.Err(err),
		)
		return biz.Team{}, biz.Session{}, err
	}

	a.publishSpiritTeamAssembled(ctx, spiritSessionID, result.Team, result.Session, params.Mode, params.TaskDescription)

	a.lg.Info("精灵团队组装完成",
		loggateway.StepID("spirit.team.assemble_done"),
		loggateway.Str("team_id", result.Team.ID),
		loggateway.Str("session_id", result.Session.ID),
	)

	return result.Team, result.Session, nil
}

func (a *SpiritTeamAssembler) publishSpiritTeamAssembled(ctx context.Context, spiritSessionID string, team biz.Team, teamSession biz.Session, mode, taskDesc string) {
	if a.bus == nil {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeSpiritTeamAssembled, "spirit-team-assembler", spiritSessionID)
	env.TeamID = team.ID
	env.Metadata = map[string]any{
		"team_id":      team.ID,
		"team_name":    team.DisplayName,
		"session_id":   teamSession.ID,
		"mode":         mode,
		"task_summary": biz.TruncateRunes(taskDesc, 200),
	}
	a.bus.Publish(ctx, env)
}

func (o *ChatOrchestrator) executeSpiritTeamTurn(
	ctx context.Context,
	spiritSess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	flow *event.TraceEmitter,
	unlock func(),
) (biz.ChatMessage, biz.ChatMessage, error) {
	if o.spiritAssembler == nil {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("SPIRIT", "spirit team assembler not configured")
	}

	team, teamSession, err := o.buildSpiritTeam(ctx, spiritSess, input, ag)
	if err != nil {
		unlock()
		flow.LogError("chat.spirit.build_team", "精灵 Team 构建失败", event.P("error", err.Error()))
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.spirit.build_team", "精灵 Team 已构建",
		event.P("team_id", team.ID),
		event.P("team_session_id", teamSession.ID),
	)

	teamInput := input
	teamInput.SessionID = teamSession.ID
	teamInput.TeamID = team.ID

	return o.executeTeamTurnViaHooks(ctx, teamSession, teamInput, flow, unlock)
}

func (o *ChatOrchestrator) buildSpiritTeam(
	ctx context.Context,
	spiritSess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
) (biz.Team, biz.Session, error) {
	params := biz.SpiritTeamParams{
		SpiritSessionID: spiritSess.ID,
		TaskDescription: strings.TrimSpace(input.Content),
		AgentIDs:        []string{ag.ID},
		Mode:            "coordinator",
	}
	return o.spiritAssembler.AssembleTeam(ctx, params)
}
