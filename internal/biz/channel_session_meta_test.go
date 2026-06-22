package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestBuildChannelSessionMetadataJSON(t *testing.T) {
	cases := []struct {
		name         string
		ch           biz.Channel
		platform     string
		peerID       string
		peerKey      string
		wantContains []string
	}{
		{
			name:         "basic channel",
			ch:           biz.Channel{ID: "ch1", Key: "mykey", ConfigJSON: `{"type":"telegram"}`},
			platform:     "telegram",
			peerID:       "p1",
			peerKey:      "pk1",
			wantContains: []string{`"source":"channel"`, `"channel_id":"ch1"`, `"channel_key":"mykey"`, `"platform":"telegram"`, `"peer_id":"p1"`, `"peer_key":"pk1"`},
		},
		{
			name:         "empty platform falls back to config type",
			ch:           biz.Channel{ID: "ch2", Key: "k2", ConfigJSON: `{"type":"feishu"}`},
			platform:     "",
			peerID:       "",
			peerKey:      "",
			wantContains: []string{`"platform":"feishu"`},
		},
		{
			name:         "explicit platform overrides config type",
			ch:           biz.Channel{ID: "ch3", Key: "k3", ConfigJSON: `{"type":"telegram"}`},
			platform:     "slack",
			wantContains: []string{`"platform":"slack"`},
		},
		{
			name:         "receive_mode from config",
			ch:           biz.Channel{ID: "ch4", Key: "k4", ConfigJSON: `{"type":"telegram","receive_mode":"webhook"}`},
			platform:     "telegram",
			wantContains: []string{`"receive_mode":"webhook"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.BuildChannelSessionMetadataJSON(tc.ch, tc.platform, tc.peerID, tc.peerKey)
			for _, s := range tc.wantContains {
				if !containsStr(got, s) {
					t.Fatalf("result %q does not contain %q", got, s)
				}
			}
		})
	}
}

func TestParseChannelSessionMeta(t *testing.T) {
	cases := []struct {
		name       string
		json       string
		want       bool
		wantSource string
	}{
		{"empty string", "", false, ""},
		{"whitespace", "  ", false, ""},
		{"invalid json", "not json", false, ""},
		{"non-channel source", `{"source":"other"}`, false, ""},
		{"channel source", `{"source":"channel","channel_id":"c1"}`, true, "channel"},
		{"source with spaces", `{"source":"  channel  "}`, true, "  channel  "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			meta, ok := biz.ParseChannelSessionMeta(tc.json)
			if ok != tc.want {
				t.Fatalf("ok = %v, want %v", ok, tc.want)
			}
			if ok && meta.Source != tc.wantSource {
				t.Fatalf("Source = %q, want %q", meta.Source, tc.wantSource)
			}
		})
	}
}

func TestRoutingTargetFingerprint(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		wantErr    bool
	}{
		{"empty config", `{}`, false},
		{"with routing", `{"routing":{"default_agent_id":"a1"}}`, false},
		{"invalid json", `not json`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fp, err := biz.RoutingTargetFingerprint(tc.configJSON)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if fp == "" {
				t.Fatalf("expected non-empty fingerprint")
			}
		})
	}
}

func TestRoutingTargetChanged(t *testing.T) {
	cases := []struct {
		name    string
		before  string
		after   string
		changed bool
	}{
		{"same config", `{"routing":{"default_agent_id":"a1"}}`, `{"routing":{"default_agent_id":"a1"}}`, false},
		{"different agent id", `{"routing":{"default_agent_id":"a1"}}`, `{"routing":{"default_agent_id":"a2"}}`, true},
		{"both invalid same string", `bad`, `bad`, false},
		{"both invalid different", `bad1`, `bad2`, true},
		{"empty to routing", `{}`, `{"routing":{"default_agent_id":"a1"}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.RoutingTargetChanged(tc.before, tc.after)
			if got != tc.changed {
				t.Fatalf("RoutingTargetChanged = %v, want %v", got, tc.changed)
			}
		})
	}
}

func TestListChannelsReferencingAgent(t *testing.T) {
	channels := []biz.Channel{
		{ID: "ch1", ConfigJSON: `{"routing":{"default_agent_id":"agent-1"}}`},
		{ID: "ch2", ConfigJSON: `{"routing":{"default_agent_id":"agent-2"}}`},
		{ID: "ch3", ConfigJSON: `{"routing":{"default_agent_id":"agent-1","rules":[{"agent_id":"agent-2"}]}}`},
		{ID: "ch4", ConfigJSON: `{"routing":{"default_team_id":"team-1"}}`},
	}
	cases := []struct {
		name      string
		agentID   string
		agentKey  string
		wantCount int
	}{
		{"by agent id", "agent-1", "", 2},
		{"by agent key", "", "agent-2", 2},
		{"both empty", "", "", 0},
		{"no match", "agent-3", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ListChannelsReferencingAgent(channels, tc.agentID, tc.agentKey)
			if len(got) != tc.wantCount {
				t.Fatalf("count = %d, want %d", len(got), tc.wantCount)
			}
		})
	}
}

func TestChannelRoutingReferencesAgent(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		agentID    string
		agentKey   string
		want       bool
	}{
		{"default agent match", `{"routing":{"default_agent_id":"a1"}}`, "a1", "", true},
		{"agent key match", `{"routing":{"default_agent_id":"mykey"}}`, "", "mykey", true},
		{"no match", `{"routing":{"default_agent_id":"a2"}}`, "a1", "", false},
		{"team id set but default agent still matches", `{"routing":{"default_agent_id":"a1","default_team_id":"t1","rules":[{"agent_id":"a1"}]}}`, "a1", "", true},
		{"rule match when no team", `{"routing":{"default_agent_id":"a2","rules":[{"agent_id":"a1"}]}}`, "a1", "", true},
		{"rule with team skipped", `{"routing":{"rules":[{"team_id":"t1","agent_id":"a1"}]}}`, "a1", "", false},
		{"invalid json", `bad`, "a1", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ChannelRoutingReferencesAgent(tc.configJSON, tc.agentID, tc.agentKey)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRoutingRefMatches(t *testing.T) {
	cases := []struct {
		name     string
		ref      string
		agentID  string
		agentKey string
		want     bool
	}{
		{"id match", "a1", "a1", "", true},
		{"key match", "mykey", "", "mykey", true},
		{"no match", "a1", "a2", "", false},
		{"empty ref", "", "a1", "", false},
		{"ref matches key not id", "mykey", "a1", "mykey", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.RoutingRefMatches(tc.ref, tc.agentID, tc.agentKey)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChannelTypeFromConfigJSON(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		want       string
	}{
		{"telegram", `{"type":"telegram"}`, "telegram"},
		{"with spaces", `{"type":"  feishu  "}`, "feishu"},
		{"empty type", `{"type":""}`, ""},
		{"invalid json", `bad`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ChannelTypeFromConfigJSON(tc.configJSON)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
