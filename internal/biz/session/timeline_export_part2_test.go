package session

import (
	"testing"
)

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
