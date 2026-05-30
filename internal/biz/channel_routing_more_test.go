package biz

import "testing"

func TestPeerKeyForSession(t *testing.T) {
	cases := []struct {
		dmScope string
		peerID  string
		want    string
	}{
		{"main", "ou_x", ""},
		{"per-channel-peer", "ou_x", "ou_x"},
		{"per-peer", "ou_y", "ou_y"},
		{"", "ou_z", "ou_z"},
		{"MAIN", "ou_x", ""},
		{"  main  ", "ou_x", ""},
	}
	for _, tc := range cases {
		got := PeerKeyForSession(tc.dmScope, tc.peerID)
		if got != tc.want {
			t.Errorf("PeerKeyForSession(%q, %q) = %q, want %q", tc.dmScope, tc.peerID, got, tc.want)
		}
	}
}

func TestMatchRoute_prefixMatch(t *testing.T) {
	r := ChannelRouting{
		DefaultAgentID: "fallback",
		Rules: []ChannelRouteRule{
			{PeerPattern: "ou_*", AgentID: "special"},
		},
	}
	a, _ := MatchRoute(r, "ou_123")
	if a != "special" {
		t.Fatalf("prefix match: got %q", a)
	}
	a2, _ := MatchRoute(r, "other")
	if a2 != "fallback" {
		t.Fatalf("fallback: got %q", a2)
	}
}

func TestMatchRoute_exactMatch(t *testing.T) {
	r := ChannelRouting{
		DefaultAgentID: "fallback",
		Rules: []ChannelRouteRule{
			{PeerPattern: "ou_exact", AgentID: "exact_agent"},
		},
	}
	a, _ := MatchRoute(r, "ou_exact")
	if a != "exact_agent" {
		t.Fatalf("exact match: got %q", a)
	}
}

func TestMatchRoute_emptyPattern(t *testing.T) {
	r := ChannelRouting{
		DefaultAgentID: "fallback",
		Rules: []ChannelRouteRule{
			{PeerPattern: "", AgentID: "ignored"},
		},
	}
	a, _ := MatchRoute(r, "any")
	if a != "fallback" {
		t.Fatalf("empty pattern should skip: got %q", a)
	}
}

func TestMatchRoute_teamRule(t *testing.T) {
	r := ChannelRouting{
		DefaultAgentID: "fallback",
		Rules: []ChannelRouteRule{
			{PeerPattern: "ou_*", TeamID: "team1"},
		},
	}
	_, team := MatchRoute(r, "ou_123")
	if team != "team1" {
		t.Fatalf("team match: got %q", team)
	}
}

func TestRoutingRefMatches(t *testing.T) {
	cases := []struct {
		ref      string
		agentID  string
		agentKey string
		want     bool
	}{
		{"uuid-1", "uuid-1", "bot-1", true},
		{"bot-1", "uuid-1", "bot-1", true},
		{"other", "uuid-1", "bot-1", false},
		{"", "uuid-1", "bot-1", false},
		{"uuid-1", "", "", false},
	}
	for _, tc := range cases {
		got := routingRefMatches(tc.ref, tc.agentID, tc.agentKey)
		if got != tc.want {
			t.Errorf("routingRefMatches(%q, %q, %q) = %v, want %v", tc.ref, tc.agentID, tc.agentKey, got, tc.want)
		}
	}
}

func TestChannelRoutingReferencesAgent(t *testing.T) {
	cases := []struct {
		configJSON string
		agentID    string
		agentKey   string
		want       bool
	}{
		{`{"routing":{"default_agent_id":"bot-1"}}`, "uuid-1", "bot-1", true},
		{`{"routing":{"default_agent_id":"bot-1"}}`, "uuid-2", "bot-2", false},
		{`{"routing":{"default_team_id":"team-1"}}`, "uuid-1", "bot-1", false},
		{`{"routing":{"default_agent_id":"bot-1","rules":[{"peer_pattern":"ou_*","agent_id":"bot-2"}]}}`, "uuid-2", "bot-2", true},
		{`invalid`, "uuid-1", "bot-1", false},
	}
	for _, tc := range cases {
		got := channelRoutingReferencesAgent(tc.configJSON, tc.agentID, tc.agentKey)
		if got != tc.want {
			t.Errorf("channelRoutingReferencesAgent(%q, %q, %q) = %v, want %v", tc.configJSON, tc.agentID, tc.agentKey, got, tc.want)
		}
	}
}

func TestChannelTypeFromConfigJSON(t *testing.T) {
	cases := []struct {
		configJSON string
		want       string
	}{
		{`{"type":"feishu"}`, "feishu"},
		{`{"type":"  dingtalk  "}`, "dingtalk"},
		{`{}`, ""},
		{`invalid`, ""},
	}
	for _, tc := range cases {
		got := channelTypeFromConfigJSON(tc.configJSON)
		if got != tc.want {
			t.Errorf("channelTypeFromConfigJSON(%q) = %q, want %q", tc.configJSON, got, tc.want)
		}
	}
}
