package team

import (
	"encoding/json"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
)

func preview(s string, max int) string {
	return strings.TrimSpace(runesTruncate(strings.TrimSpace(s), max))
}

func runesTruncate(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func topologyJSON(def Definition) string {
	ids := make([]string, 0, len(def.Members))
	for _, m := range EnabledMembers(def) {
		ids = append(ids, m.AgentID)
	}
	b, _ := json.Marshal(map[string]any{"member_order": ids, "mode": def.Mode})
	return string(b)
}

func extractOpts(req *chatv1.SendChatMessageRequest) (dialogMode, prov, mod string, attN int) {
	if req == nil {
		return "", "", "", 0
	}
	o := req.GetOptions()
	if o == nil {
		return "", "", "", 0
	}
	return strings.TrimSpace(o.GetDialogMode()), strings.TrimSpace(o.GetProvider()), strings.TrimSpace(o.GetModel()), len(o.Attachments)
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
