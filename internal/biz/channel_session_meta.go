package biz

import (
	"encoding/json"
	"strings"
)

// ChannelSessionMeta is stored in sessions.metadata_json for channel-originated sessions.
type ChannelSessionMeta struct {
	Source      string `json:"source"`
	ChannelID   string `json:"channel_id"`
	ChannelKey  string `json:"channel_key"`
	Platform    string `json:"platform"`
	PeerID      string `json:"peer_id,omitempty"`
	PeerKey     string `json:"peer_key,omitempty"`
	ReceiveMode string `json:"receive_mode,omitempty"`
}

// BuildChannelSessionMetadataJSON returns metadata_json for a channel-bound session.
func BuildChannelSessionMetadataJSON(ch Channel, platform, peerID, peerKey string) string {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		platform = channelTypeFromConfigJSON(ch.ConfigJSON)
	}
	receiveMode := ""
	if cfg, err := ch.ParseConfig(); err == nil {
		receiveMode = strings.TrimSpace(cfg.ReceiveMode)
	}
	meta := ChannelSessionMeta{
		Source:      "channel",
		ChannelID:   strings.TrimSpace(ch.ID),
		ChannelKey:  strings.TrimSpace(ch.Key),
		Platform:    platform,
		PeerID:      strings.TrimSpace(peerID),
		PeerKey:     strings.TrimSpace(peerKey),
		ReceiveMode: receiveMode,
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return `{"source":"channel"}`
	}
	return string(b)
}

// ParseChannelSessionMeta extracts channel session metadata; ok=false when not a channel session.
func ParseChannelSessionMeta(metadataJSON string) (ChannelSessionMeta, bool) {
	metadataJSON = strings.TrimSpace(metadataJSON)
	if metadataJSON == "" {
		return ChannelSessionMeta{}, false
	}
	var meta ChannelSessionMeta
	if err := json.Unmarshal([]byte(metadataJSON), &meta); err != nil {
		return ChannelSessionMeta{}, false
	}
	if strings.TrimSpace(meta.Source) != "channel" {
		return ChannelSessionMeta{}, false
	}
	return meta, true
}

// RoutingTargetFingerprint returns a stable string for routing target comparison.
func RoutingTargetFingerprint(configJSON string) (string, error) {
	r, err := ParseChannelRouting(configJSON)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(struct {
		DefaultAgentID string             `json:"default_agent_id"`
		DefaultTeamID  string             `json:"default_team_id"`
		DMScope        string             `json:"dm_scope"`
		Rules          []ChannelRouteRule `json:"rules"`
	}{
		DefaultAgentID: r.DefaultAgentID,
		DefaultTeamID:  r.DefaultTeamID,
		DMScope:        r.DMScope,
		Rules:          r.Rules,
	})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// RoutingTargetChanged reports whether inbound routing semantics changed between two configs.
func RoutingTargetChanged(beforeJSON, afterJSON string) bool {
	before, err1 := RoutingTargetFingerprint(beforeJSON)
	after, err2 := RoutingTargetFingerprint(afterJSON)
	if err1 != nil || err2 != nil {
		return beforeJSON != afterJSON
	}
	return before != after
}

// ListChannelsReferencingAgent returns channel instances whose routing points at agentID (UUID or agent_key).
func ListChannelsReferencingAgent(channels []Channel, agentID, agentKey string) []Channel {
	agentID = strings.TrimSpace(agentID)
	agentKey = strings.TrimSpace(agentKey)
	if agentID == "" && agentKey == "" {
		return nil
	}
	var out []Channel
	for _, ch := range channels {
		if channelRoutingReferencesAgent(ch.ConfigJSON, agentID, agentKey) {
			out = append(out, ch)
		}
	}
	return out
}

func channelRoutingReferencesAgent(configJSON, agentID, agentKey string) bool {
	r, err := ParseChannelRouting(configJSON)
	if err != nil {
		return false
	}
	if routingRefMatches(r.DefaultAgentID, agentID, agentKey) {
		return true
	}
	if strings.TrimSpace(r.DefaultTeamID) != "" {
		return false
	}
	for _, rule := range r.Rules {
		if strings.TrimSpace(rule.TeamID) != "" {
			continue
		}
		if routingRefMatches(rule.AgentID, agentID, agentKey) {
			return true
		}
	}
	return false
}

func routingRefMatches(ref, agentID, agentKey string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	return ref == agentID || (agentKey != "" && ref == agentKey)
}

func channelTypeFromConfigJSON(configJSON string) string {
	cfg, err := parseChannelConfig(configJSON)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.Type)
}
