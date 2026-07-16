package biz

// StepToActivity converts a v2 Step to the v1 Activity shape.
//
// Temporary adapter for ListActivities / session ActivityLister call sites
// that still speak the Activity DTO. Persistence is steps_v2 only.
//
// Field mapping notes:
//   - Kind: StepKind values overlap ActivityKind for the kinds that
//     matter for v1 backward compat (thinking/action/reply/notice/confirm).
//     StepKind "error" has no v1 equivalent and is preserved as the
//     literal string; downstream v1 code treats unknown kinds leniently.
//   - Status: StepStatus values are a subset of ActivityStatus values,
//     so a direct string cast is safe.
//   - Meta: populated with is_final/notice_type/agent_key (the kind-
//     specific metadata that v1 carried in Activity.Meta).
//   - DurationMs: computed from StartedAt + CompletedAt.
//   - TaskID/Version: v1 Activity has no TaskID/Version field, so these
//     are dropped (Task 15 will delete v1 entirely).
func StepToActivity(s Step) Activity {
	meta := make(map[string]any, 3)
	if s.IsFinal {
		meta["is_final"] = true
	}
	if s.NoticeType != "" {
		meta["notice_type"] = s.NoticeType
	}
	if s.AuthorAgentKey != "" {
		meta["agent_key"] = s.AuthorAgentKey
	}
	var durationMs int64
	if !s.StartedAt.IsZero() && s.CompletedAt != nil && !s.CompletedAt.Before(s.StartedAt) {
		durationMs = s.CompletedAt.Sub(s.StartedAt).Milliseconds()
	}
	return Activity{
		ID:              s.ID,
		Kind:            ActivityKind(s.Kind),
		Status:          ActivityStatus(s.Status),
		SessionID:       s.SessionID,
		TurnID:          s.TurnID,
		Timestamp:       s.StartedAt,
		DurationMs:      durationMs,
		Seq:             s.Seq,
		Content:         s.Content,
		Reasoning:       s.Reasoning,
		ToolName:        s.ToolName,
		ToolCallID:      s.ToolCallID,
		ToolArguments:   string(s.ToolArgs),
		ToolResult:      string(s.ToolResult),
		ToolDurationMs:  s.ToolDurationMs,
		ToolErrorCode:   s.ToolErrorCode,
		SpiritSessionID: s.SpiritSessionID,
		AgentKey:        s.AuthorAgentKey,
		Meta:            meta,
	}
}
