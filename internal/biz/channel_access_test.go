package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestParseChannelAccessPolicy(t *testing.T) {
	cases := []struct {
		name       string
		configJSON string
		wantMention bool
		wantUserLen int
		wantGroupLen int
		wantErr     bool
	}{
		{
			name:       "empty config",
			configJSON: `{}`,
			wantMention: false,
			wantUserLen: 0,
			wantGroupLen: 0,
		},
		{
			name:       "require_mention true",
			configJSON: `{"config":{"require_mention":true}}`,
			wantMention: true,
		},
		{
			name:       "require_mention string true",
			configJSON: `{"config":{"require_mention":"true"}}`,
			wantMention: true,
		},
		{
			name:       "require_mention string 1",
			configJSON: `{"config":{"require_mention":"1"}}`,
			wantMention: true,
		},
		{
			name:       "require_mention false",
			configJSON: `{"config":{"require_mention":false}}`,
			wantMention: false,
		},
		{
			name:       "allowed_user_ids comma string",
			configJSON: `{"config":{"allowed_user_ids":"u1,u2"}}`,
			wantUserLen: 2,
		},
		{
			name:       "allowed_user_ids array",
			configJSON: `{"config":{"allowed_user_ids":["a1","a2","a3"]}}`,
			wantUserLen: 3,
		},
		{
			name:       "allowed_group_ids comma string",
			configJSON: `{"config":{"allowed_group_ids":"g1,g2"}}`,
			wantGroupLen: 2,
		},
		{
			name:       "deny all user ids",
			configJSON: `{"config":{"allowed_user_ids":"0"}}`,
			wantUserLen: 1,
		},
		{
			name:       "invalid json",
			configJSON: `not json`,
			wantErr:    true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy, err := biz.ParseChannelAccessPolicy(tc.configJSON)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if policy.RequireMention != tc.wantMention {
				t.Fatalf("RequireMention = %v, want %v", policy.RequireMention, tc.wantMention)
			}
			if len(policy.AllowedUserIDs) != tc.wantUserLen {
				t.Fatalf("len(AllowedUserIDs) = %d, want %d", len(policy.AllowedUserIDs), tc.wantUserLen)
			}
			if len(policy.AllowedGroupIDs) != tc.wantGroupLen {
				t.Fatalf("len(AllowedGroupIDs) = %d, want %d", len(policy.AllowedGroupIDs), tc.wantGroupLen)
			}
		})
	}
}

func TestChannelAccessPolicy_Allows(t *testing.T) {
	cases := []struct {
		name    string
		policy  biz.ChannelAccessPolicy
		ctx     biz.InboundAccessContext
		want    bool
		wantMsg string
	}{
		{
			name: "empty policy allows all",
			policy: biz.ChannelAccessPolicy{},
			ctx:    biz.InboundAccessContext{UserIDs: []string{"u1"}, IsGroup: false},
			want:   true,
		},
		{
			name: "require mention group not mentioned",
			policy: biz.ChannelAccessPolicy{RequireMention: true},
			ctx:    biz.InboundAccessContext{IsGroup: true, Mentioned: false},
			want:   false,
			wantMsg: "group message requires @mention",
		},
		{
			name: "require mention group mentioned",
			policy: biz.ChannelAccessPolicy{RequireMention: true},
			ctx:    biz.InboundAccessContext{IsGroup: true, Mentioned: true},
			want:   true,
		},
		{
			name: "require mention DM ok",
			policy: biz.ChannelAccessPolicy{RequireMention: true},
			ctx:    biz.InboundAccessContext{IsGroup: false, Mentioned: false},
			want:   true,
		},
		{
			name: "allowed user ids match",
			policy: biz.ChannelAccessPolicy{
				AllowedUserIDs: map[string]struct{}{"u1": {}, "u2": {}},
			},
			ctx:  biz.InboundAccessContext{UserIDs: []string{"u1"}},
			want: true,
		},
		{
			name: "allowed user ids no match",
			policy: biz.ChannelAccessPolicy{
				AllowedUserIDs: map[string]struct{}{"u1": {}},
			},
			ctx:     biz.InboundAccessContext{UserIDs: []string{"u3"}},
			want:    false,
			wantMsg: "sender not in allowed_user_ids",
		},
		{
			name: "deny all user ids sentinel",
			policy: biz.ChannelAccessPolicy{
				AllowedUserIDs: map[string]struct{}{"0": {}},
			},
			ctx:     biz.InboundAccessContext{UserIDs: []string{"u1"}},
			want:    false,
			wantMsg: "all users denied by allowed_user_ids",
		},
		{
			name: "allowed group ids match",
			policy: biz.ChannelAccessPolicy{
				AllowedGroupIDs: map[string]struct{}{"g1": {}},
			},
			ctx:  biz.InboundAccessContext{IsGroup: true, GroupID: "g1"},
			want: true,
		},
		{
			name: "allowed group ids no match",
			policy: biz.ChannelAccessPolicy{
				AllowedGroupIDs: map[string]struct{}{"g1": {}},
			},
			ctx:     biz.InboundAccessContext{IsGroup: true, GroupID: "g2"},
			want:    false,
			wantMsg: "group not in allowed_group_ids",
		},
		{
			name: "deny all group ids sentinel",
			policy: biz.ChannelAccessPolicy{
				AllowedGroupIDs: map[string]struct{}{"0": {}},
			},
			ctx:     biz.InboundAccessContext{IsGroup: true, GroupID: "g1"},
			want:    false,
			wantMsg: "all groups denied by allowed_group_ids",
		},
		{
			name: "group ids not checked for DM",
			policy: biz.ChannelAccessPolicy{
				AllowedGroupIDs: map[string]struct{}{"g1": {}},
			},
			ctx:  biz.InboundAccessContext{IsGroup: false, GroupID: "g2"},
			want: true,
		},
		{
			name: "empty group id in group chat with allowlist",
			policy: biz.ChannelAccessPolicy{
				AllowedGroupIDs: map[string]struct{}{"g1": {}},
			},
			ctx:     biz.InboundAccessContext{IsGroup: true, GroupID: ""},
			want:    false,
			wantMsg: "group not in allowed_group_ids",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, msg := tc.policy.Allows(tc.ctx)
			if got != tc.want {
				t.Fatalf("Allows = %v, want %v", got, tc.want)
			}
			if !got && msg != tc.wantMsg {
				t.Fatalf("msg = %q, want %q", msg, tc.wantMsg)
			}
		})
	}
}

func TestParseIDAllowlist(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want int
	}{
		{"nil", nil, 0},
		{"empty string", "", 0},
		{"comma string", "a,b,c", 3},
		{"json array string", `["x","y"]`, 2},
		{"slice of any", []any{"p", "q"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ParseIDAllowlist(tc.raw)
			if len(got) != tc.want {
				t.Fatalf("len = %d, want %d", len(got), tc.want)
			}
		})
	}
}

func TestParseStringList(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want []string
	}{
		{"nil", nil, nil},
		{"empty string", "", nil},
		{"whitespace string", "  ", nil},
		{"comma string", "a,b", []string{"a", "b"}},
		{"json array string", `["x","y"]`, []string{"x", "y"}},
		{"slice of any", []any{"p", "q"}, []string{"p", "q"}},
		{"slice of string", []string{"a", "b"}, []string{"a", "b"}},
		{"int value", 42, []string{"42"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ParseStringList(tc.raw)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d, got %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseConfigBool(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want bool
	}{
		{"bool true", true, true},
		{"bool false", false, false},
		{"string true", "true", true},
		{"string 1", "1", true},
		{"string yes", "yes", true},
		{"string on", "on", true},
		{"string false", "false", false},
		{"string 0", "0", false},
		{"int value", 1, false},
		{"nil", nil, false},
		{"string TRUE uppercase", "TRUE", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.ParseConfigBool(tc.raw)
			if got != tc.want {
				t.Fatalf("ParseConfigBool(%v) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestMatchesAnyID(t *testing.T) {
	allow := map[string]struct{}{"u1": {}, "u2": {}}
	cases := []struct {
		name string
		ids  []string
		want bool
	}{
		{"match first", []string{"u1", "u3"}, true},
		{"match second", []string{"u3", "u2"}, true},
		{"no match", []string{"u3", "u4"}, false},
		{"empty ids", []string{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MatchesAnyID(tc.ids, allow)
			if got != tc.want {
				t.Fatalf("MatchesAnyID = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMatchesID(t *testing.T) {
	allow := map[string]struct{}{"abc": {}}
	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"match", "abc", true},
		{"no match", "xyz", false},
		{"empty id", "", false},
		{"whitespace id", "  ", false},
		{"trimmed match", "  abc  ", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.MatchesID(tc.id, allow)
			if got != tc.want {
				t.Fatalf("MatchesID = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSplitCommaList(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want []string
	}{
		{"single", "a", []string{"a"}},
		{"comma separated", "a,b,c", []string{"a", "b", "c"}},
		{"with spaces", " a , b , c ", []string{"a", "b", "c"}},
		{"trailing comma", "a,b,", []string{"a", "b"}},
		{"empty parts skipped", ",a,,b,", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.SplitCommaList(tc.s)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestNormalizeStringList(t *testing.T) {
	cases := []struct {
		name  string
		items []any
		want  []string
	}{
		{"nil items", nil, nil},
		{"empty items", []any{}, nil},
		{"mixed", []any{"a", "  b  ", "", "<nil>", 42}, []string{"a", "b", "42"}},
		{"all empty", []any{"", "  ", "<nil>"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := biz.NormalizeStringList(tc.items)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d, got %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
