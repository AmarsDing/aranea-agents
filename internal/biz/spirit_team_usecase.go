package biz

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type SpiritTeamParams struct {
	SpiritSessionID string
	TaskDescription string
	AgentIDs        []string
	Mode            string
}

type SpiritTeamResult struct {
	Team    Team
	Session Session
}

type SpiritTeamUsecase struct {
	teamUC    *TeamUsecase
	sessionUC *SessionUsecase
}

func NewSpiritTeamUsecase(teamUC *TeamUsecase, sessionUC *SessionUsecase) *SpiritTeamUsecase {
	return &SpiritTeamUsecase{teamUC: teamUC, sessionUC: sessionUC}
}

func (u *SpiritTeamUsecase) AssembleTeam(ctx context.Context, params SpiritTeamParams) (SpiritTeamResult, error) {
	spiritSessionID := strings.TrimSpace(params.SpiritSessionID)
	if spiritSessionID == "" {
		return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT", "spirit_session_id is required")
	}
	taskDesc := strings.TrimSpace(params.TaskDescription)
	if taskDesc == "" {
		return SpiritTeamResult{}, kerrors.BadRequest("SPIRIT", "task_description is required")
	}
	mode := strings.TrimSpace(params.Mode)
	if mode == "" {
		mode = "coordinator"
	}

	defJSON := buildSpiritTeamDefinitionJSON(mode, params.AgentIDs)

	team, err := u.teamUC.Create(ctx, Team{
		TeamKey:         "spirit_" + spiritSessionID,
		DisplayName:     TruncateRunes(taskDesc, 64),
		Status:          "active",
		SpiritSessionID: spiritSessionID,
		TaskDescription: taskDesc,
		AutoCreated:     true,
		DefinitionJSON:  defJSON,
	})
	if err != nil {
		return SpiritTeamResult{}, kerrors.InternalServer("SPIRIT", "create team: "+err.Error())
	}

	teamSession, err := u.sessionUC.Create(ctx, Session{
		OwnerType:       "team",
		TeamID:          team.ID,
		ParentSessionID: spiritSessionID,
		RootSessionID:   spiritSessionID,
		AgentDepth:      1,
		Title:           TruncateRunes(taskDesc, 128),
	})
	if err != nil {
		return SpiritTeamResult{}, kerrors.InternalServer("SPIRIT", "create team session: "+err.Error())
	}

	if err := u.appendChildSessionID(ctx, spiritSessionID, teamSession.ID); err != nil {
		return SpiritTeamResult{}, err
	}

	return SpiritTeamResult{Team: team, Session: teamSession}, nil
}

func (u *SpiritTeamUsecase) appendChildSessionID(ctx context.Context, spiritSessionID, childSessionID string) error {
	sess, err := u.sessionUC.Get(ctx, spiritSessionID)
	if err != nil {
		return err
	}
	var meta map[string]any
	if strings.TrimSpace(sess.MetadataJSON) != "" {
		if err := json.Unmarshal([]byte(sess.MetadataJSON), &meta); err != nil {
			meta = make(map[string]any)
		}
	}
	if meta == nil {
		meta = make(map[string]any)
	}
	var children []string
	if raw, ok := meta["child_session_ids"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					children = append(children, s)
				}
			}
		}
	}
	children = append(children, childSessionID)
	meta["child_session_ids"] = children
	updated, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	metaStr := string(updated)
	_, err = u.sessionUC.Update(ctx, spiritSessionID, SessionUpdateFields{
		MetadataJSON: &metaStr,
	})
	return err
}

func buildSpiritTeamDefinitionJSON(mode string, agentIDs []string) string {
	type member struct {
		AgentID string `json:"agent_id"`
		Role    string `json:"role"`
		Enabled *bool  `json:"enabled"`
	}
	members := make([]member, 0, len(agentIDs))
	for i, id := range agentIDs {
		role := "worker"
		enabled := true
		if i == 0 && mode == "coordinator" {
			role = "synthesizer"
		}
		members = append(members, member{
			AgentID: strings.TrimSpace(id),
			Role:    role,
			Enabled: &enabled,
		})
	}
	def := map[string]any{
		"version":            2,
		"mode":               mode,
		"runtime_engine":     "graph",
		"team_graph_runtime": true,
		"members":            members,
		"max_concurrency":    2,
		"timeout_seconds":    600,
	}
	out, err := json.Marshal(def)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func TruncateRunes(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-3]) + "..."
}
