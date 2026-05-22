package biz

import (
	"testing"
)

func TestChannelAccessPolicyEmptyAllowsAll(t *testing.T) {
	p := ChannelAccessPolicy{}
	ok, reason := p.Allows(InboundAccessContext{UserIDs: []string{"u1"}, IsGroup: true, GroupID: "g1"})
	if !ok || reason != "" {
		t.Fatalf("want allow, got ok=%v reason=%q", ok, reason)
	}
}

func TestChannelAccessPolicyUserAllowlist(t *testing.T) {
	p := ChannelAccessPolicy{AllowedUserIDs: map[string]struct{}{"ou_a": {}}}
	ok, _ := p.Allows(InboundAccessContext{UserIDs: []string{"ou_a"}})
	if !ok {
		t.Fatal("expected allowed user")
	}
	ok, reason := p.Allows(InboundAccessContext{UserIDs: []string{"ou_b"}})
	if ok || reason != "sender not in allowed_user_ids" {
		t.Fatalf("got ok=%v reason=%q", ok, reason)
	}
}

func TestChannelAccessPolicyGroupAllowlist(t *testing.T) {
	p := ChannelAccessPolicy{AllowedGroupIDs: map[string]struct{}{"oc_g1": {}}}
	ok, _ := p.Allows(InboundAccessContext{UserIDs: []string{"ou_a"}, IsGroup: true, GroupID: "oc_g1"})
	if !ok {
		t.Fatal("expected allowed group")
	}
	ok, reason := p.Allows(InboundAccessContext{UserIDs: []string{"ou_a"}, IsGroup: true, GroupID: "oc_g2"})
	if ok || reason != "group not in allowed_group_ids" {
		t.Fatalf("got ok=%v reason=%q", ok, reason)
	}
	ok, _ = p.Allows(InboundAccessContext{UserIDs: []string{"ou_a"}, IsGroup: false, GroupID: ""})
	if !ok {
		t.Fatal("dm should not be restricted by group list")
	}
}

func TestChannelAccessPolicyRequireMention(t *testing.T) {
	p := ChannelAccessPolicy{RequireMention: true}
	ok, reason := p.Allows(InboundAccessContext{IsGroup: true, Mentioned: false})
	if ok || reason != "group message requires @mention" {
		t.Fatalf("got ok=%v reason=%q", ok, reason)
	}
}

func TestParseChannelAccessPolicyFromConfig(t *testing.T) {
	raw := `{"type":"feishu","config":{"require_mention":true,"allowed_user_ids":["ou_a","ou_b"],"allowed_group_ids":"oc_g1,oc_g2"}}`
	p, err := ParseChannelAccessPolicy(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !p.RequireMention || len(p.AllowedUserIDs) != 2 || len(p.AllowedGroupIDs) != 2 {
		t.Fatalf("unexpected policy %#v", p)
	}
}

func TestInboundAccessContextFromEvent(t *testing.T) {
	p := ChannelAccessPolicy{RequireMention: true}
	ok, reason := p.Allows(InboundAccessContext{
		UserIDs:   []string{"ou_x"},
		GroupID:   "oc_g",
		IsGroup:   true,
		Mentioned: true,
	})
	if !ok || reason != "" {
		t.Fatalf("got ok=%v reason=%q", ok, reason)
	}
}
