package service

import (
	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	"encoding/json"
	"time"
)

func toProtoActivity(a biz.Activity) *v1.Activity {
	dependsOn := a.DependsOn
	if dependsOn == nil {
		dependsOn = []string{}
	}
	dependsOnJSON, _ := json.Marshal(dependsOn)
	// AF-BE-17: serialize Meta so the frontend can read is_final / member_id /
	// as_display via the ListActivities RPC. Previously Meta was dropped here,
	// causing historical replies loaded via API to lose their is_final flag
	// and render as "intermediate" replies in the chat UI.
	var metaJSON string
	if len(a.Meta) > 0 {
		if b, err := json.Marshal(a.Meta); err == nil {
			metaJSON = string(b)
		}
	}
	return &v1.Activity{
		Id:               a.ID,
		Kind:             string(a.Kind),
		Status:           string(a.Status),
		SessionId:        a.SessionID,
		TurnId:           a.TurnID,
		ParentActivityId: a.ParentActivityID,
		Timestamp:        a.Timestamp.Format(time.RFC3339Nano),
		DurationMs:       a.DurationMs,
		Seq:              a.Seq,
		Content:          a.Content,
		Reasoning:        a.Reasoning,
		ToolName:         a.ToolName,
		ToolCallId:       a.ToolCallID,
		ToolArguments:    a.ToolArguments,
		ToolResult:       a.ToolResult,
		ToolDurationMs:   a.ToolDurationMs,
		ToolErrorCode:    a.ToolErrorCode,
		ChildBoardId:     a.ChildBoardID,
		SpiritSessionId:  a.SpiritSessionID,
		TeamId:           a.TeamID,
		DagNodeId:        a.DagNodeID,
		DependsOnJson:    string(dependsOnJSON),
		AgentKey:         a.AgentKey,
		AgentName:        a.AgentName,
		Collapsed:        a.Collapsed,
		Label:            a.Label,
		PromptTokens:     a.PromptTokens,
		CompletionTokens: a.CompletionTokens,
		MetaJson:         metaJSON,
	}
}
