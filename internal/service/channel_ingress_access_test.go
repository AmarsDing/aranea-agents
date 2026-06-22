package service

import (
	"reflect"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
)

func TestMetaBool(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"empty", "", false},
		{"spaces", "   ", false},
		{"one", "1", true},
		{"zero", "0", false},
		{"true", "true", true},
		{"True", "True", true},
		{"TRUE", "TRUE", true},
		{"false", "false", false},
		{"False", "False", false},
		{"yes", "yes", true},
		{"Yes", "Yes", true},
		{"YES", "YES", true},
		{"no", "no", false},
		{"on", "on", true},
		{"On", "On", true},
		{"ON", "ON", true},
		{"off", "off", false},
		{"random", "maybe", false},
		{"spaced_true", " true ", true},
		{"spaced_yes", " yes ", true},
		{"spaced_on", " on ", true},
		{"spaced_1", " 1 ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metaBool(tt.input); got != tt.want {
				t.Errorf("metaBool(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestUniqueNonEmptyStrings(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, nil},
		{"all_empty", []string{"", " ", "  "}, nil},
		{"single", []string{"a"}, []string{"a"}},
		{"duplicates", []string{"a", "a", "a"}, []string{"a"}},
		{"mixed", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"with_spaces", []string{" a ", "b", " a"}, []string{"a", "b"}},
		{"empty_between", []string{"a", "", "b"}, []string{"a", "b"}},
		{"space_only_between", []string{"a", "   ", "b"}, []string{"a", "b"}},
		{"preserves_order", []string{"c", "a", "b"}, []string{"c", "a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uniqueNonEmptyStrings(tt.parts...)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("uniqueNonEmptyStrings(%v) = %v, want %v", tt.parts, got, tt.want)
			}
		})
	}
}

func TestInboundAccessContextFromEvent(t *testing.T) {
	tests := []struct {
		name string
		ev   port.InboundEvent
		want biz.InboundAccessContext
	}{
		{
			name: "nil_meta_dm",
			ev: port.InboundEvent{
				PeerID: "user-1",
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"user-1"},
				GroupID:   "",
				IsGroup:   false,
				Mentioned: false,
			},
		},
		{
			name: "dm_with_user_ids",
			ev: port.InboundEvent{
				PeerID:       "peer-1",
				OutboundMeta: map[string]string{"sender_open_id": "open-1", "sender_user_id": "uid-1", "user_id": "uid-1"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer-1", "open-1", "uid-1"},
				GroupID:   "",
				IsGroup:   false,
				Mentioned: false,
			},
		},
		{
			name: "group_chat_type",
			ev: port.InboundEvent{
				PeerID:       "peer-1",
				OutboundMeta: map[string]string{"chat_type": "group", "chat_id": "grp-1"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer-1"},
				GroupID:   "grp-1",
				IsGroup:   true,
				Mentioned: false,
			},
		},
		{
			name: "group_conversation_type",
			ev: port.InboundEvent{
				PeerID:       "peer-1",
				OutboundMeta: map[string]string{"conversation_type": "Group", "chat_id": "grp-2"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer-1"},
				GroupID:   "grp-2",
				IsGroup:   true,
				Mentioned: false,
			},
		},
		{
			name: "group_without_chat_id_uses_peer_id",
			ev: port.InboundEvent{
				PeerID:       "peer-group",
				OutboundMeta: map[string]string{"chat_type": "group"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer-group"},
				GroupID:   "peer-group",
				IsGroup:   true,
				Mentioned: false,
			},
		},
		{
			name: "mentioned_true",
			ev: port.InboundEvent{
				PeerID:       "user-1",
				OutboundMeta: map[string]string{"mentioned": "true"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"user-1"},
				GroupID:   "",
				IsGroup:   false,
				Mentioned: true,
			},
		},
		{
			name: "bot_mentioned_true",
			ev: port.InboundEvent{
				PeerID:       "user-1",
				OutboundMeta: map[string]string{"bot_mentioned": "1"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"user-1"},
				GroupID:   "",
				IsGroup:   false,
				Mentioned: true,
			},
		},
		{
			name: "group_mentions_non_empty",
			ev: port.InboundEvent{
				PeerID:       "user-1",
				OutboundMeta: map[string]string{"chat_type": "group", "chat_id": "g1", "mentions": "@bot"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"user-1"},
				GroupID:   "g1",
				IsGroup:   true,
				Mentioned: true,
			},
		},
		{
			name: "group_mentions_empty_no_mention",
			ev: port.InboundEvent{
				PeerID:       "user-1",
				OutboundMeta: map[string]string{"chat_type": "group", "chat_id": "g1", "mentions": "  "},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"user-1"},
				GroupID:   "g1",
				IsGroup:   true,
				Mentioned: false,
			},
		},
		{
			name: "dm_not_group",
			ev: port.InboundEvent{
				PeerID:       "user-1",
				OutboundMeta: map[string]string{"chat_type": "private"},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"user-1"},
				GroupID:   "",
				IsGroup:   false,
				Mentioned: false,
			},
		},
		{
			name: "full_scenario",
			ev: port.InboundEvent{
				PeerID: "peer-1",
				OutboundMeta: map[string]string{
					"sender_open_id": "open-1",
					"sender_user_id": "uid-1",
					"user_id":        "uid-2",
					"chat_id":        "grp-1",
					"chat_type":      "group",
					"mentioned":      "true",
				},
			},
			want: biz.InboundAccessContext{
				UserIDs:   []string{"peer-1", "open-1", "uid-1", "uid-2"},
				GroupID:   "grp-1",
				IsGroup:   true,
				Mentioned: true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inboundAccessContextFromEvent(tt.ev)
			if !reflect.DeepEqual(got.UserIDs, tt.want.UserIDs) {
				t.Errorf("UserIDs = %v, want %v", got.UserIDs, tt.want.UserIDs)
			}
			if got.GroupID != tt.want.GroupID {
				t.Errorf("GroupID = %q, want %q", got.GroupID, tt.want.GroupID)
			}
			if got.IsGroup != tt.want.IsGroup {
				t.Errorf("IsGroup = %v, want %v", got.IsGroup, tt.want.IsGroup)
			}
			if got.Mentioned != tt.want.Mentioned {
				t.Errorf("Mentioned = %v, want %v", got.Mentioned, tt.want.Mentioned)
			}
		})
	}
}
