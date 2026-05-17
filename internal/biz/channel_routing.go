package biz

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// ChannelRouting is parsed from config_json.routing.
type ChannelRouting struct {
	DefaultAgentID string             `json:"default_agent_id"`
	DefaultTeamID  string             `json:"default_team_id"`
	DMScope        string             `json:"dm_scope"`
	Rules          []ChannelRouteRule `json:"rules"`
}

// ChannelRouteRule maps a peer pattern to a target id (agent UUID/key or team UUID).
type ChannelRouteRule struct {
	PeerPattern string `json:"peer_pattern"`
	AgentID     string `json:"agent_id"`
	TeamID      string `json:"team_id"`
}

// ParseChannelRouting extracts routing from config_json. Omitted fields default safely.
func ParseChannelRouting(configJSON string) (ChannelRouting, error) {
	cfg, err := parseChannelConfig(configJSON)
	if err != nil {
		return ChannelRouting{}, err
	}
	if cfg.Routing == nil {
		return ChannelRouting{DMScope: "per-channel-peer"}, nil
	}
	raw, err := json.Marshal(cfg.Routing)
	if err != nil {
		return ChannelRouting{}, err
	}
	var r ChannelRouting
	if err := json.Unmarshal(raw, &r); err != nil {
		return ChannelRouting{}, channelValidationError("routing must be valid JSON object")
	}
	r.DefaultAgentID = strings.TrimSpace(r.DefaultAgentID)
	r.DefaultTeamID = strings.TrimSpace(r.DefaultTeamID)
	r.DMScope = strings.TrimSpace(strings.ToLower(r.DMScope))
	if r.DMScope == "" {
		r.DMScope = "per-channel-peer"
	}
	return r, nil
}

// PeerKeyForSession returns the stable peer_key for channel_peer_session rows.
// dm_scope: main → empty; per-channel-peer / per-peer → peerID (MVP: per-peer treated like per-channel-peer within one channel).
func PeerKeyForSession(dmScope, peerID string) string {
	peerID = strings.TrimSpace(peerID)
	switch strings.ToLower(strings.TrimSpace(dmScope)) {
	case "main":
		return ""
	default:
		return peerID
	}
}

// MatchRoute picks a routing rule by peer_id glob (filepath.Match).
func MatchRoute(r ChannelRouting, peerID string) (agentID, teamID string) {
	peerID = strings.TrimSpace(peerID)
	for _, rule := range r.Rules {
		pat := strings.TrimSpace(rule.PeerPattern)
		if pat == "" {
			continue
		}
		ok, err := filepath.Match(pat, peerID)
		if err != nil {
			continue
		}
		if ok {
			return strings.TrimSpace(rule.AgentID), strings.TrimSpace(rule.TeamID)
		}
	}
	return r.DefaultAgentID, r.DefaultTeamID
}

// ResolveChannelTarget resolves routing to session owner_type + agent_id or team_id.
// agent identifiers may be UUID or agent_key.
func ResolveChannelTarget(ctx context.Context, agents AgentRepository, teams TeamRepository, r ChannelRouting, peerID string) (ownerType, agentID, teamID string, err error) {
	ra, rt := MatchRoute(r, peerID)
	teamID = strings.TrimSpace(rt)
	if teamID != "" {
		if teams == nil {
			return "", "", "", errors.InternalServer("CHANNEL", "team repository not configured")
		}
		if _, e := teams.GetTeamByID(ctx, teamID); e != nil {
			if errors.IsNotFound(e) || e == sql.ErrNoRows {
				return "", "", "", errors.NotFound("TEAM", "routing team not found")
			}
			return "", "", "", e
		}
		return "team", "", teamID, nil
	}
	agentRef := strings.TrimSpace(ra)
	if agentRef == "" {
		return "", "", "", errors.BadRequest("CHANNEL", "routing has no default_agent_id or team for this peer")
	}
	if ag, e := agents.GetAgentByID(ctx, agentRef); e == nil && strings.TrimSpace(ag.ID) != "" {
		return "agent", ag.ID, "", nil
	}
	if ag, e := agents.GetAgentByAgentKey(ctx, agentRef); e == nil && strings.TrimSpace(ag.ID) != "" {
		return "agent", ag.ID, "", nil
	}
	return "", "", "", errors.NotFound("AGENT", "routing target agent not found")
}
