package tools

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// ---------------------------------------------------------------------------
// get_team_deliverable (F6, Phase 11): structured result retrieval for Spirit
// ---------------------------------------------------------------------------

// SpiritTeamDeliverablePort provides team listing + deliverable full-text read
// for the get_team_deliverable tool. Implemented by service.SpiritTeamAssembler.
// Stability:evolving
type SpiritTeamDeliverablePort interface {
	ListAllTeams(ctx context.Context, spiritSessionID string) ([]biz.Team, error)
	ReadUpstreamDeliverable(ctx context.Context, readerSessionID, teamID string, maxChars int) (biz.UpstreamDeliverableContent, error)
}

// GetTeamDeliverableInput is the input for the get_team_deliverable tool.
type GetTeamDeliverableInput struct {
	TeamID   string `json:"team_id,omitempty" jsonschema:"description=目标团队 ID。留空时返回本会话全部团队清单（team_id/名称/状态/任务），用于定位团队后再次调用。"`
	MaxChars int    `json:"max_chars,omitempty" jsonschema:"description=返回内容最大字符数，留空使用默认预算。"`
}

// TeamBriefView is one team's brief in the listing output.
type TeamBriefView struct {
	TeamID   string `json:"team_id"`
	TeamName string `json:"team_name"`
	Status   string `json:"status"`
	Task     string `json:"task"`
}

// GetTeamDeliverableOutput is the output for the get_team_deliverable tool.
// Exactly one of Teams / Content is populated. Read failures surface in the
// Error field (never as a tool-level error) so the LLM can read the reason
// and recover — e.g. pick another team or wait for completion.
type GetTeamDeliverableOutput struct {
	Teams     []TeamBriefView `json:"teams,omitempty"`
	TeamID    string          `json:"team_id,omitempty"`
	TeamName  string          `json:"team_name,omitempty"`
	Status    string          `json:"status,omitempty"`
	Content   string          `json:"content,omitempty"`
	SizeChars int             `json:"size_chars,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// NewGetTeamDeliverableTool creates the get_team_deliverable tool (F6).
// The spirit uses it to retrieve a team's structured deliverable in ONE call,
// replacing read_session_history archaeology + get_deliverable topic guessing.
// Reads use an empty readerSessionID, which exempts the InputContract
// validation that applies to downstream-team readers (resolveReaderContracts).
func NewGetTeamDeliverableTool(port SpiritTeamDeliverablePort) *trpcfunction.FunctionTool[GetTeamDeliverableInput, GetTeamDeliverableOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input GetTeamDeliverableInput) (GetTeamDeliverableOutput, error) {
			spiritSessionID := spiritSessionIDFromCtx(ctx)
			if spiritSessionID == "" {
				return GetTeamDeliverableOutput{}, apierror.BadRequest(apierror.DomainSpirit, "spirit session id not found in context")
			}

			teamID := strings.TrimSpace(input.TeamID)

			// Empty team_id → list teams to guide selection.
			if teamID == "" {
				teams, err := port.ListAllTeams(ctx, spiritSessionID)
				if err != nil {
					return GetTeamDeliverableOutput{Error: "查询团队清单失败: " + err.Error()}, nil
				}
				out := GetTeamDeliverableOutput{Teams: make([]TeamBriefView, 0, len(teams))}
				for _, t := range teams {
					out.Teams = append(out.Teams, TeamBriefView{
						TeamID:   t.ID,
						TeamName: t.DisplayName,
						Status:   t.Status,
						Task:     t.TaskDescription,
					})
				}
				return out, nil
			}

			// Resolve team name/status context from the session's team list
			// (best effort; the read itself validates team existence).
			var teamName, teamStatus string
			if teams, err := port.ListAllTeams(ctx, spiritSessionID); err == nil {
				for _, t := range teams {
					if t.ID == teamID {
						teamName, teamStatus = t.DisplayName, t.Status
						break
					}
				}
			}

			content, err := port.ReadUpstreamDeliverable(ctx, "", teamID, input.MaxChars)
			if err != nil {
				return GetTeamDeliverableOutput{
					TeamID:   teamID,
					TeamName: teamName,
					Status:   teamStatus,
					Error:    err.Error(),
				}, nil
			}
			return GetTeamDeliverableOutput{
				TeamID:    teamID,
				TeamName:  teamName,
				Status:    teamStatus,
				Content:   content.Content,
				SizeChars: content.SizeChars,
				Truncated: content.Truncated,
			}, nil
		},
		trpcfunction.WithName("get_team_deliverable"),
		trpcfunction.WithDescription("读取指定团队的交付物全文（结构化执行结果）。team_id 留空时列出本会话全部团队（team_id/名称/状态/任务）供选择；指定 team_id 时返回该团队交付物内容。团队未完成或读取失败时 error 字段说明原因。需要团队执行结果时优先调用本工具，不要用 read_session_history 翻聊天记录。"),
	)
}
