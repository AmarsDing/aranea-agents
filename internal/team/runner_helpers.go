package team

import (
	"encoding/json"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/pkg/strutil"
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
	return strutil.FirstNonEmpty(vals...)
}

// mergeTeamUserADKMetaJSON adds audit fields for the text sent to ADK vs content_markdown shown in chat.
func mergeTeamUserADKMetaJSON(userOpts string, displayContent, adkSendText string) (string, error) {
	displayContent = strings.TrimSpace(displayContent)
	adkSendText = strings.TrimSpace(adkSendText)
	var opts map[string]any
	if strings.TrimSpace(userOpts) == "" {
		opts = map[string]any{}
	} else if err := json.Unmarshal([]byte(userOpts), &opts); err != nil {
		return userOpts, err
	}
	sendLen := len([]rune(adkSendText))
	opts["team_adk_user_display_len"] = len([]rune(displayContent))
	opts["team_adk_user_send_len"] = sendLen
	opts["team_adk_user_send_differs_from_display"] = adkSendText != displayContent
	// Plan / audit aliases (same values as team_* send fields).
	opts["adk_user_turn_length"] = sendLen
	if adkSendText != "" {
		pr := runesTruncate(adkSendText, 240)
		opts["team_adk_user_send_preview"] = pr
		opts["adk_user_text_preview"] = pr
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return userOpts, err
	}
	return string(out), nil
}
