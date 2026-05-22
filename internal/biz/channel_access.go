package biz

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ChannelAccessPolicy controls who may trigger Agent turns on a channel instance.
// Fields live in config_json.config (see docs/需求/17 channel.md §6).
type ChannelAccessPolicy struct {
	RequireMention  bool
	AllowedUserIDs  map[string]struct{}
	AllowedGroupIDs map[string]struct{}
}

// InboundAccessContext is normalized from port.InboundEvent for access checks.
type InboundAccessContext struct {
	UserIDs   []string
	GroupID   string
	IsGroup   bool
	Mentioned bool
}

const channelAccessDenyAll = "0"

// ParseChannelAccessPolicy reads config_json.config access fields.
func ParseChannelAccessPolicy(configJSON string) (ChannelAccessPolicy, error) {
	cfg, err := parseChannelConfig(configJSON)
	if err != nil {
		return ChannelAccessPolicy{}, err
	}
	raw := cfg.Config
	policy := ChannelAccessPolicy{
		RequireMention:  parseConfigBool(raw["require_mention"]),
		AllowedUserIDs:  parseIDAllowlist(raw["allowed_user_ids"]),
		AllowedGroupIDs: parseIDAllowlist(raw["allowed_group_ids"]),
	}
	return policy, nil
}

// Allows reports whether an inbound message may proceed to routing / Agent turn.
//
// Semantics (Aranea, stricter than MuseBot OR-combo):
//   - Empty allowlist on a dimension → that dimension does not restrict.
//   - Non-empty allowed_user_ids → sender must match one listed ID.
//   - Non-empty allowed_group_ids → group chats must match one listed chat/conversation ID.
//   - require_mention → group chats must carry a platform @ mention (Feishu/钉钉等).
//   - Sentinel "0" in a list → deny all on that dimension (MuseBot compat).
func (p ChannelAccessPolicy) Allows(in InboundAccessContext) (bool, string) {
	if p.RequireMention && in.IsGroup && !in.Mentioned {
		return false, "group message requires @mention"
	}
	if len(p.AllowedUserIDs) > 0 {
		if _, denyAll := p.AllowedUserIDs[channelAccessDenyAll]; denyAll {
			return false, "all users denied by allowed_user_ids"
		}
		if !matchesAnyID(in.UserIDs, p.AllowedUserIDs) {
			return false, "sender not in allowed_user_ids"
		}
	}
	if in.IsGroup && len(p.AllowedGroupIDs) > 0 {
		if _, denyAll := p.AllowedGroupIDs[channelAccessDenyAll]; denyAll {
			return false, "all groups denied by allowed_group_ids"
		}
		groupID := strings.TrimSpace(in.GroupID)
		if groupID == "" || !matchesID(groupID, p.AllowedGroupIDs) {
			return false, "group not in allowed_group_ids"
		}
	}
	return true, ""
}

func parseIDAllowlist(raw any) map[string]struct{} {
	ids := parseStringList(raw)
	if len(ids) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func parseStringList(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		if strings.HasPrefix(s, "[") {
			var arr []any
			if json.Unmarshal([]byte(s), &arr) == nil {
				return normalizeStringList(arr)
			}
		}
		return splitCommaList(s)
	case []any:
		return normalizeStringList(v)
	case []string:
		return normalizeStringList(toAnySlice(v))
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" || s == "<nil>" {
			return nil
		}
		return []string{s}
	}
}

func toAnySlice(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

func normalizeStringList(items []any) []string {
	var out []string
	for _, item := range items {
		s := strings.TrimSpace(fmt.Sprint(item))
		if s == "" || s == "<nil>" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func splitCommaList(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func parseConfigBool(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func matchesAnyID(ids []string, allow map[string]struct{}) bool {
	for _, id := range ids {
		if matchesID(id, allow) {
			return true
		}
	}
	return false
}

func matchesID(id string, allow map[string]struct{}) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	_, ok := allow[id]
	return ok
}
