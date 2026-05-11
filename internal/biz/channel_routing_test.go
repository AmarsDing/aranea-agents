package biz

import "testing"

func TestParseChannelRouting_defaults(t *testing.T) {
	r, err := ParseChannelRouting(`{"type":"feishu","config":{"app_id":"x"},"routing":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.DMScope != "per-channel-peer" {
		t.Fatalf("dm_scope %q", r.DMScope)
	}
}

func TestMatchRoute_glob(t *testing.T) {
	r := ChannelRouting{
		DefaultAgentID: "fallback",
		Rules: []ChannelRouteRule{
			{PeerPattern: "oc_*", AgentID: "special"},
		},
	}
	a, _ := MatchRoute(r, "oc_123")
	if a != "special" {
		t.Fatal(a)
	}
	a2, _ := MatchRoute(r, "other")
	if a2 != "fallback" {
		t.Fatal(a2)
	}
}
