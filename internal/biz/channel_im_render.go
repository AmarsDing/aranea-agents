package biz

import (
	"encoding/json"
	"strings"
)

const (
	ChannelIMRenderModeReplyOnly              = "reply_only"
	ChannelIMRenderModeTranscript             = "transcript"
	ChannelIMRenderModeTranscriptWithReasoning = "transcript_with_reasoning"

	ChannelIMToolDetailOff          = "off"
	ChannelIMToolDetailLabel        = "label"
	ChannelIMToolDetailLabelSummary = "label_summary"

	ChannelIMTeamModeOff    = "off"
	ChannelIMTeamModeInline = "inline"
	ChannelIMTeamModeSteps  = "steps"

	ChannelIMToolCardModeOff         = "off"
	ChannelIMToolCardModeFeishuAppend = "feishu_append"

	defaultIMReasoningMaxRunes = 500
	defaultIMMaxPreviewRunes   = 4000
)

// ChannelIMRenderPolicy controls how Channel turns are projected to IM plain text.
type ChannelIMRenderPolicy struct {
	Mode              string
	ShowReasoning     bool
	ReasoningMaxRunes int
	ToolDetail        string
	TeamMode          string
	MaxPreviewRunes   int
	HeartbeatEnabled  bool
	HeartbeatQuietSec int
	HeartbeatMessage  string
	ToolCardMode      string
	SplitOverflow     bool
}

// ParseChannelIMRenderPolicy reads IM render settings from channel config_json.
// Legacy progress_mode is mapped when im_render_mode is absent.
func ParseChannelIMRenderPolicy(configJSON string, ltCfg ChannelLongTaskConfig) ChannelIMRenderPolicy {
	policy := ChannelIMRenderPolicy{
		Mode:              ChannelIMRenderModeReplyOnly,
		ReasoningMaxRunes: defaultIMReasoningMaxRunes,
		ToolDetail:        ChannelIMToolDetailLabelSummary,
		TeamMode:          ChannelIMTeamModeOff,
		MaxPreviewRunes:   defaultIMMaxPreviewRunes,
		HeartbeatQuietSec: ltCfg.ProgressQuietSec,
		HeartbeatMessage:  ltCfg.HeartbeatMessage,
	}
	if policy.HeartbeatQuietSec <= 0 {
		policy.HeartbeatQuietSec = 20
	}
	if strings.TrimSpace(policy.HeartbeatMessage) == "" {
		policy.HeartbeatMessage = defaultChannelHeartbeatMsg
	}

	var env struct {
		Config struct {
			IMRenderMode         *string `json:"im_render_mode"`
			IMShowReasoning      *bool   `json:"im_show_reasoning"`
			IMReasoningMaxChars  *int    `json:"im_reasoning_max_chars"`
			IMToolDetail         *string `json:"im_tool_detail"`
			IMTeamMode           *string `json:"im_team_mode"`
			IMMaxPreviewChars    *int    `json:"im_max_preview_chars"`
			IMToolCardMode       *string `json:"im_tool_card_mode"`
			IMSplitOverflow      *bool   `json:"im_split_overflow"`
			ProgressMode         *string `json:"progress_mode"`
			ProgressQuietSec     *int    `json:"progress_quiet_sec"`
			HeartbeatMessage     *string `json:"heartbeat_message"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(defaultJSON(configJSON)), &env) != nil {
		applyLegacyProgressMode(&policy, ltCfg)
		return policy
	}

	if env.Config.IMRenderMode != nil {
		policy.Mode = normalizeIMRenderMode(*env.Config.IMRenderMode)
	}
	if env.Config.IMShowReasoning != nil {
		policy.ShowReasoning = *env.Config.IMShowReasoning
	}
	if env.Config.IMReasoningMaxChars != nil && *env.Config.IMReasoningMaxChars > 0 {
		policy.ReasoningMaxRunes = *env.Config.IMReasoningMaxChars
	}
	if env.Config.IMToolDetail != nil {
		policy.ToolDetail = normalizeIMToolDetail(*env.Config.IMToolDetail)
	}
	if env.Config.IMTeamMode != nil {
		policy.TeamMode = normalizeIMTeamMode(*env.Config.IMTeamMode)
	}
	if env.Config.IMMaxPreviewChars != nil && *env.Config.IMMaxPreviewChars > 0 {
		policy.MaxPreviewRunes = *env.Config.IMMaxPreviewChars
	}
	if env.Config.IMToolCardMode != nil {
		policy.ToolCardMode = normalizeIMToolCardMode(*env.Config.IMToolCardMode)
	}
	if env.Config.IMSplitOverflow != nil {
		policy.SplitOverflow = *env.Config.IMSplitOverflow
	}
	if env.Config.ProgressQuietSec != nil && *env.Config.ProgressQuietSec >= 0 {
		policy.HeartbeatQuietSec = *env.Config.ProgressQuietSec
	}
	if env.Config.HeartbeatMessage != nil {
		policy.HeartbeatMessage = strings.TrimSpace(*env.Config.HeartbeatMessage)
	}

	if env.Config.IMRenderMode == nil {
		applyLegacyProgressMode(&policy, ltCfg)
	}
	if policy.Mode == ChannelIMRenderModeTranscriptWithReasoning {
		policy.ShowReasoning = true
	}
	policy.HeartbeatEnabled = policy.Mode != ChannelIMRenderModeReplyOnly &&
		(policy.HeartbeatQuietSec > 0 || ltCfg.ProgressEnabled())
	return policy
}

func applyLegacyProgressMode(policy *ChannelIMRenderPolicy, ltCfg ChannelLongTaskConfig) {
	if policy == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(ltCfg.ProgressMode)) {
	case "text":
		policy.Mode = ChannelIMRenderModeTranscript
		if policy.ToolDetail == "" {
			policy.ToolDetail = ChannelIMToolDetailLabel
		}
	case "steps":
		policy.Mode = ChannelIMRenderModeTranscript
		policy.TeamMode = ChannelIMTeamModeSteps
	default:
		// reply_only remains default
	}
}

// ChannelACKDeferredToPreview reports whether accept should skip a separate ACK delivery.
func ChannelACKDeferredToPreview(configJSON, platform string) bool {
	if !ChannelStreamingEnabled(configJSON) {
		return false
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	switch platform {
	case "feishu", "lark", "slack", "telegram":
		return true
	default:
		return false
	}
}

func normalizeIMRenderMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ChannelIMRenderModeTranscript:
		return ChannelIMRenderModeTranscript
	case ChannelIMRenderModeTranscriptWithReasoning:
		return ChannelIMRenderModeTranscriptWithReasoning
	default:
		return ChannelIMRenderModeReplyOnly
	}
}

func normalizeIMToolDetail(detail string) string {
	switch strings.ToLower(strings.TrimSpace(detail)) {
	case ChannelIMToolDetailOff:
		return ChannelIMToolDetailOff
	case ChannelIMToolDetailLabel:
		return ChannelIMToolDetailLabel
	default:
		return ChannelIMToolDetailLabelSummary
	}
}

func normalizeIMTeamMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ChannelIMTeamModeInline:
		return ChannelIMTeamModeInline
	case ChannelIMTeamModeSteps:
		return ChannelIMTeamModeSteps
	default:
		return ChannelIMTeamModeOff
	}
}

func normalizeIMToolCardMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ChannelIMToolCardModeFeishuAppend:
		return ChannelIMToolCardModeFeishuAppend
	default:
		return ChannelIMToolCardModeOff
	}
}
