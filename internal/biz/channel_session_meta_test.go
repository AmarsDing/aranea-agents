package biz

import "testing"

func TestRoutingTargetChanged(t *testing.T) {
	before := `{"routing":{"default_agent_id":"a1","dm_scope":"per-channel-peer"}}`
	afterSame := `{"routing":{"default_agent_id":"a1","dm_scope":"per-channel-peer"}}`
	afterNew := `{"routing":{"default_team_id":"t1","dm_scope":"main"}}`
	if RoutingTargetChanged(before, afterSame) {
		t.Fatal("expected unchanged routing")
	}
	if !RoutingTargetChanged(before, afterNew) {
		t.Fatal("expected routing change")
	}
}

func TestListChannelsReferencingAgent(t *testing.T) {
	channels := []Channel{
		{ID: "c1", Key: "lark", ConfigJSON: `{"routing":{"default_agent_id":"bot-1"}}`},
		{ID: "c2", Key: "other", ConfigJSON: `{"routing":{"default_team_id":"team-1"}}`},
	}
	got := ListChannelsReferencingAgent(channels, "uuid-1", "bot-1")
	if len(got) != 1 || got[0].ID != "c1" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseChannelSessionMeta(t *testing.T) {
	raw := BuildChannelSessionMetadataJSON(Channel{ID: "ch1", Key: "feishu_main", ConfigJSON: `{"type":"feishu"}`}, "feishu", "ou_x", "ou_x")
	meta, ok := ParseChannelSessionMeta(raw)
	if !ok || meta.ChannelKey != "feishu_main" || meta.Platform != "feishu" {
		t.Fatalf("meta=%+v ok=%v", meta, ok)
	}
}
