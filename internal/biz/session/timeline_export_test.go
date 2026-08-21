package session

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSkillTimelineItem(t *testing.T) {
	tests := []struct {
		name string
		run  SkillInvocationView
		want SessionTimelineItem
	}{
		{
			name: "basic skill invocation",
			run: SkillInvocationView{
				ID:               "skill-1",
				SkillName:        "CodeReview",
				SkillVersion:     "v2.1",
				AgentID:          "agent-1",
				AgentDisplayName: "Reviewer",
				Status:           "success",
				StartedAt:        "2026-05-21T10:00:00Z",
				DurationMS:       350,
				InputPreview:     "review this code",
				OutputPreview:    "looks good",
			},
			want: SessionTimelineItem{
				ID:         "skill-1",
				Kind:       "skill",
				Side:       "right",
				Title:      "CodeReview",
				Subtitle:   "v2.1",
				ActorID:    "agent-1",
				ActorName:  "Reviewer",
				Status:     "success",
				OccurredAt: "2026-05-21T10:00:00Z",
				DurationMS: 350,
				Preview:    "review this code",
				Tags:       []string{"Skill"},
			},
		},
		{
			name: "empty skill name falls back to default",
			run: SkillInvocationView{
				ID:           "skill-2",
				SkillVersion: "v1",
				Status:       "success",
				StartedAt:    "2026-05-21T11:00:00Z",
			},
			want: SessionTimelineItem{
				ID:         "skill-2",
				Kind:       "skill",
				Side:       "right",
				Title:      "技能调用",
				TitleKey:   "skill_call",
				Subtitle:   "v1",
				Status:     "success",
				OccurredAt: "2026-05-21T11:00:00Z",
				Tags:       []string{"Skill"},
			},
		},
		{
			name: "empty status defaults to success",
			run: SkillInvocationView{
				ID:        "skill-3",
				SkillName: "Deploy",
				StartedAt: "2026-05-21T12:00:00Z",
			},
			want: SessionTimelineItem{
				ID:         "skill-3",
				Kind:       "skill",
				Side:       "right",
				Title:      "Deploy",
				Status:     "success",
				OccurredAt: "2026-05-21T12:00:00Z",
				Tags:       []string{"Skill"},
			},
		},
		{
			name: "error status preserved",
			run: SkillInvocationView{
				ID:            "skill-4",
				SkillName:     "Build",
				Status:        "error",
				ErrorMessage:  "build failed",
				StartedAt:     "2026-05-21T13:00:00Z",
				DurationMS:    100,
				InputPreview:  "build input",
				OutputPreview: "",
			},
			want: SessionTimelineItem{
				ID:         "skill-4",
				Kind:       "skill",
				Side:       "right",
				Title:      "Build",
				Status:     "error",
				OccurredAt: "2026-05-21T13:00:00Z",
				DurationMS: 100,
				Preview:    "build input",
				Tags:       []string{"Skill"},
			},
		},
		{
			name: "preview falls back to output when input empty",
			run: SkillInvocationView{
				ID:            "skill-5",
				SkillName:     "Analyze",
				Status:        "success",
				StartedAt:     "2026-05-21T14:00:00Z",
				InputPreview:  "",
				OutputPreview: "analysis result",
			},
			want: SessionTimelineItem{
				ID:         "skill-5",
				Kind:       "skill",
				Side:       "right",
				Title:      "Analyze",
				Status:     "success",
				OccurredAt: "2026-05-21T14:00:00Z",
				Preview:    "analysis result",
				Tags:       []string{"Skill"},
			},
		},
		{
			name: "preview falls back to error message when both empty",
			run: SkillInvocationView{
				ID:           "skill-6",
				SkillName:    "Test",
				Status:       "error",
				StartedAt:    "2026-05-21T15:00:00Z",
				ErrorMessage: "timeout",
			},
			want: SessionTimelineItem{
				ID:         "skill-6",
				Kind:       "skill",
				Side:       "right",
				Title:      "Test",
				Status:     "error",
				OccurredAt: "2026-05-21T15:00:00Z",
				Preview:    "timeout",
				Tags:       []string{"Skill"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SkillTimelineItem(tt.run)
			if got.ID != tt.want.ID {
				t.Errorf("ID = %q, want %q", got.ID, tt.want.ID)
			}
			if got.Kind != tt.want.Kind {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.want.Kind)
			}
			if got.Side != tt.want.Side {
				t.Errorf("Side = %q, want %q", got.Side, tt.want.Side)
			}
			if got.Title != tt.want.Title {
				t.Errorf("Title = %q, want %q", got.Title, tt.want.Title)
			}
			if got.Subtitle != tt.want.Subtitle {
				t.Errorf("Subtitle = %q, want %q", got.Subtitle, tt.want.Subtitle)
			}
			if got.ActorID != tt.want.ActorID {
				t.Errorf("ActorID = %q, want %q", got.ActorID, tt.want.ActorID)
			}
			if got.ActorName != tt.want.ActorName {
				t.Errorf("ActorName = %q, want %q", got.ActorName, tt.want.ActorName)
			}
			if got.Status != tt.want.Status {
				t.Errorf("Status = %q, want %q", got.Status, tt.want.Status)
			}
			if got.OccurredAt != tt.want.OccurredAt {
				t.Errorf("OccurredAt = %q, want %q", got.OccurredAt, tt.want.OccurredAt)
			}
			if got.DurationMS != tt.want.DurationMS {
				t.Errorf("DurationMS = %d, want %d", got.DurationMS, tt.want.DurationMS)
			}
			if got.Preview != tt.want.Preview {
				t.Errorf("Preview = %q, want %q", got.Preview, tt.want.Preview)
			}
			if len(got.Tags) != len(tt.want.Tags) {
				t.Errorf("Tags len = %d, want %d", len(got.Tags), len(tt.want.Tags))
			} else {
				for i := range got.Tags {
					if got.Tags[i] != tt.want.Tags[i] {
						t.Errorf("Tags[%d] = %q, want %q", i, got.Tags[i], tt.want.Tags[i])
					}
				}
			}
		})
	}
}

func TestMarshalTimelineDetail(t *testing.T) {
	tests := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{
			name:  "simple key-value",
			value: map[string]any{"key": "value"},
			want:  `{"key":"value"}`,
		},
		{
			name:  "multiple keys sorted",
			value: map[string]any{"b": 2, "a": 1},
			want:  `{"a":1,"b":2}`,
		},
		{
			name:  "empty map",
			value: map[string]any{},
			want:  `{}`,
		},
		{
			name:  "nested value",
			value: map[string]any{"nested": map[string]any{"inner": true}},
			want:  `{"nested":{"inner":true}}`,
		},
		{
			name:  "null value",
			value: map[string]any{"nil": nil},
			want:  `{"nil":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarshalTimelineDetail(tt.value)
			var gotParsed, wantParsed any
			if err := json.Unmarshal([]byte(got), &gotParsed); err != nil {
				t.Fatalf("got is not valid JSON: %q, err: %v", got, err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantParsed); err != nil {
				t.Fatalf("want is not valid JSON: %q, err: %v", tt.want, err)
			}
			gotBytes, _ := json.Marshal(gotParsed)
			wantBytes, _ := json.Marshal(wantParsed)
			if string(gotBytes) != string(wantBytes) {
				t.Errorf("MarshalTimelineDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSessionMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		sess     Session
		messages []ChatMessage
		timeline SessionTimeline
		check    func(t *testing.T, got string)
	}{
		{
			name: "session with title and messages",
			sess: Session{
				ID:           "sess-1",
				Title:        "My Session",
				OwnerType:    "agent",
				AgentID:      "agent-1",
				MessageCount: 2,
				TotalTokens:  100,
			},
			messages: []ChatMessage{
				{Role: "user", TurnNumber: 1, ContentMarkdown: "hello"},
				{Role: "assistant", TurnNumber: 1, ContentMarkdown: "hi there"},
			},
			timeline: SessionTimeline{
				Items: []SessionTimelineItem{
					{Kind: "message", Title: "User", Status: "ok", OccurredAt: "2026-05-21T10:00:00Z", Preview: "hello"},
				},
			},
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "# My Session") {
					t.Error("markdown should contain session title heading")
				}
				if !strings.Contains(got, "sess-1") {
					t.Error("markdown should contain session ID")
				}
				if !strings.Contains(got, "hello") {
					t.Error("markdown should contain user message")
				}
				if !strings.Contains(got, "hi there") {
					t.Error("markdown should contain assistant message")
				}
				if !strings.Contains(got, "## Messages") {
					t.Error("markdown should contain Messages heading")
				}
				if !strings.Contains(got, "## Timeline") {
					t.Error("markdown should contain Timeline heading")
				}
			},
		},
		{
			name: "empty title produces heading with no name",
			sess: Session{
				ID:        "sess-2",
				OwnerType: "agent",
			},
			messages: nil,
			timeline: SessionTimeline{},
			check: func(t *testing.T, got string) {
				if !strings.HasPrefix(got, "# ") {
					t.Error("markdown should start with heading prefix")
				}
				if strings.Contains(got, "Untitled Session") {
					t.Error("Untitled Session fallback should not appear when sb.Len() != 1")
				}
			},
		},
		{
			name: "session with summary",
			sess: Session{
				ID:           "sess-3",
				Title:        "Summary Test",
				OwnerType:    "agent",
				Summary:      "This is a summary of the session",
				MessageCount: 1,
				TotalTokens:  50,
			},
			messages: []ChatMessage{},
			timeline: SessionTimeline{},
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "## Summary") {
					t.Error("markdown should contain Summary heading")
				}
				if !strings.Contains(got, "This is a summary of the session") {
					t.Error("markdown should contain summary content")
				}
			},
		},
		{
			name: "session with team ID",
			sess: Session{
				ID:        "sess-4",
				Title:     "Team Session",
				OwnerType: "team",
				TeamID:    "team-1",
			},
			messages: nil,
			timeline: SessionTimeline{},
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "team-1") {
					t.Error("markdown should contain team ID")
				}
			},
		},
		{
			name: "timeline items with preview",
			sess: Session{
				ID:        "sess-5",
				Title:     "Timeline Preview",
				OwnerType: "agent",
			},
			messages: nil,
			timeline: SessionTimeline{
				Items: []SessionTimelineItem{
					{Kind: "tool", Title: "Search", Status: "success", OccurredAt: "2026-05-21T10:00:00Z", Preview: "search result preview"},
					{Kind: "skill", Title: "Analyze", Status: "ok", OccurredAt: "2026-05-21T10:01:00Z", Preview: ""},
				},
			},
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "search result preview") {
					t.Error("markdown should contain tool preview")
				}
				if !strings.Contains(got, "**tool**") {
					t.Error("markdown should contain tool kind in bold")
				}
				if !strings.Contains(got, "**skill**") {
					t.Error("markdown should contain skill kind in bold")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSessionMarkdown(tt.sess, tt.messages, tt.timeline)
			tt.check(t, got)
		})
	}
}

func TestNormalizeSessionSearch(t *testing.T) {
	tests := []struct {
		name string
		q    SessionSearchQuery
		want SessionSearchQuery
	}{
		{
			name: "PageSize computes Limit and Offset",
			q: SessionSearchQuery{
				Page:     3,
				PageSize: 25,
			},
			want: SessionSearchQuery{
				Page:     3,
				PageSize: 25,
				Limit:    25,
				Offset:   50,
			},
		},
		{
			name: "Page zero with PageSize resets Offset to zero",
			q: SessionSearchQuery{
				Page:     0,
				PageSize: 10,
			},
			want: SessionSearchQuery{
				Page:     0,
				PageSize: 10,
				Limit:    10,
				Offset:   0,
			},
		},
		{
			name: "negative Page with PageSize resets Offset to zero",
			q: SessionSearchQuery{
				Page:     -1,
				PageSize: 10,
			},
			want: SessionSearchQuery{
				Page:     -1,
				PageSize: 10,
				Limit:    10,
				Offset:   0,
			},
		},
		{
			name: "zero Limit and no PageSize defaults to 20",
			q: SessionSearchQuery{
				Limit: 0,
			},
			want: SessionSearchQuery{
				Limit:  20,
				Offset: 0,
			},
		},
		{
			name: "negative Limit defaults to 20",
			q: SessionSearchQuery{
				Limit: -5,
			},
			want: SessionSearchQuery{
				Limit:  20,
				Offset: 0,
			},
		},
		{
			name: "Limit over 100 within max preserved",
			q: SessionSearchQuery{
				Limit: 200,
			},
			want: SessionSearchQuery{
				Limit:  200,
				Offset: 0,
			},
		},
		{
			name: "Limit over max clamped to max",
			q: SessionSearchQuery{
				Limit: 9999,
			},
			want: SessionSearchQuery{
				Limit:  MaxSessionSearchLimit,
				Offset: 0,
			},
		},
		{
			name: "negative Offset defaults to zero",
			q: SessionSearchQuery{
				Limit:  50,
				Offset: -10,
			},
			want: SessionSearchQuery{
				Limit:  50,
				Offset: 0,
			},
		},
		{
			name: "valid Limit and Offset preserved",
			q: SessionSearchQuery{
				Limit:  50,
				Offset: 100,
			},
			want: SessionSearchQuery{
				Limit:  50,
				Offset: 100,
			},
		},
		{
			name: "PageSize overrides Limit even when Limit set",
			q: SessionSearchQuery{
				Page:     2,
				PageSize: 30,
				Limit:    999,
				Offset:   999,
			},
			want: SessionSearchQuery{
				Page:     2,
				PageSize: 30,
				Limit:    30,
				Offset:   30,
			},
		},
		{
			name: "Limit exactly 100 preserved",
			q: SessionSearchQuery{
				Limit: 100,
			},
			want: SessionSearchQuery{
				Limit:  100,
				Offset: 0,
			},
		},
		{
			name: "Limit exactly 1 preserved",
			q: SessionSearchQuery{
				Limit: 1,
			},
			want: SessionSearchQuery{
				Limit:  1,
				Offset: 0,
			},
		},
		{
			name: "Page 1 with PageSize 20 gives Offset 0",
			q: SessionSearchQuery{
				Page:     1,
				PageSize: 20,
			},
			want: SessionSearchQuery{
				Page:     1,
				PageSize: 20,
				Limit:    20,
				Offset:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			NormalizeSessionSearch(&tt.q)
			if tt.q.Limit != tt.want.Limit {
				t.Errorf("Limit = %d, want %d", tt.q.Limit, tt.want.Limit)
			}
			if tt.q.Offset != tt.want.Offset {
				t.Errorf("Offset = %d, want %d", tt.q.Offset, tt.want.Offset)
			}
			if tt.q.PageSize != tt.want.PageSize {
				t.Errorf("PageSize = %d, want %d", tt.q.PageSize, tt.want.PageSize)
			}
			if tt.q.Page != tt.want.Page {
				t.Errorf("Page = %d, want %d", tt.q.Page, tt.want.Page)
			}
		})
	}
}

func TestSessionTimelineSummary(t *testing.T) {
	tests := []struct {
		name      string
		sess      Session
		pageItems []SessionTimelineItem
		want      SessionTimelineSummary
	}{
		{
			name: "counts from session fields",
			sess: Session{
				MessageCount:   10,
				ToolCallCount:  3,
				SkillCallCount: 2,
				MCPCallCount:   1,
			},
			pageItems: nil,
			want: SessionTimelineSummary{
				Total:        15,
				MessageCount: 10,
				ToolCount:    3,
				SkillCount:   2,
				MCPCount:     1,
			},
		},
		{
			name: "zero session counts with page items counts from items",
			sess: Session{},
			pageItems: []SessionTimelineItem{
				{Kind: "message"},
				{Kind: "message"},
				{Kind: "tool"},
				{Kind: "skill"},
				{Kind: "mcp"},
			},
			want: SessionTimelineSummary{
				Total:        5,
				MessageCount: 2,
				ToolCount:    1,
				SkillCount:   1,
				MCPCount:     1,
			},
		},
		{
			name:      "empty session and empty items",
			sess:      Session{},
			pageItems: nil,
			want: SessionTimelineSummary{
				Total:        0,
				MessageCount: 0,
				ToolCount:    0,
				SkillCount:   0,
				MCPCount:     0,
			},
		},
		{
			name: "session counts present ignores page items",
			sess: Session{
				MessageCount:   5,
				ToolCallCount:  2,
				SkillCallCount: 1,
				MCPCallCount:   0,
			},
			pageItems: []SessionTimelineItem{
				{Kind: "message"},
				{Kind: "tool"},
				{Kind: "skill"},
			},
			want: SessionTimelineSummary{
				Total:        8,
				MessageCount: 5,
				ToolCount:    2,
				SkillCount:   1,
				MCPCount:     0,
			},
		},
		{
			name: "page items with unknown kind counted only in total",
			sess: Session{},
			pageItems: []SessionTimelineItem{
				{Kind: "message"},
				{Kind: "unknown"},
			},
			want: SessionTimelineSummary{
				Total:        2,
				MessageCount: 1,
				ToolCount:    0,
				SkillCount:   0,
				MCPCount:     0,
			},
		},
		{
			name: "MCP not included in Total formula",
			sess: Session{
				MessageCount:   4,
				ToolCallCount:  2,
				SkillCallCount: 1,
				MCPCallCount:   3,
			},
			pageItems: nil,
			want: SessionTimelineSummary{
				Total:        7,
				MessageCount: 4,
				ToolCount:    2,
				SkillCount:   1,
				MCPCount:     3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildSessionTimelineSummary(tt.sess, tt.pageItems)
			if got != tt.want {
				t.Errorf("SessionTimelineSummary() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
