package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestResolveChannelAsyncGraphTarget(t *testing.T) {
	cases := []struct {
		name       string
		cfg        biz.ChannelLongTaskConfig
		wantType   string
		wantGraphID string
		wantTeamID  string
		wantErr    bool
	}{
		{
			name:     "async_team_id takes precedence",
			cfg:      biz.ChannelLongTaskConfig{AsyncTeamID: "team1", AsyncGraphID: "graph1"},
			wantType: "team_graph",
			wantTeamID: "team1",
		},
		{
			name:       "async_graph_id when no team",
			cfg:        biz.ChannelLongTaskConfig{AsyncGraphID: "graph1"},
			wantType:   "graph",
			wantGraphID: "graph1",
		},
		{
			name:    "neither configured returns error",
			cfg:     biz.ChannelLongTaskConfig{},
			wantErr: true,
		},
		{
			name:     "whitespace team id trimmed and used",
			cfg:      biz.ChannelLongTaskConfig{AsyncTeamID: "  team1  "},
			wantType: "team_graph",
			wantTeamID: "team1",
		},
		{
			name:       "whitespace graph id trimmed and used",
			cfg:        biz.ChannelLongTaskConfig{AsyncGraphID: "  graph1  "},
			wantType:   "graph",
			wantGraphID: "graph1",
		},
		{
			name:    "whitespace only treated as empty",
			cfg:     biz.ChannelLongTaskConfig{AsyncGraphID: "   "},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := biz.ResolveChannelAsyncGraphTarget(tc.cfg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.TargetType != tc.wantType {
				t.Fatalf("TargetType = %q, want %q", got.TargetType, tc.wantType)
			}
			if got.GraphID != tc.wantGraphID {
				t.Fatalf("GraphID = %q, want %q", got.GraphID, tc.wantGraphID)
			}
			if got.TeamID != tc.wantTeamID {
				t.Fatalf("TeamID = %q, want %q", got.TeamID, tc.wantTeamID)
			}
		})
	}
}
