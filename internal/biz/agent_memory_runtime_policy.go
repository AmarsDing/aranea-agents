package biz

import (
	"encoding/json"
	"sort"
	"strings"
)

const maxL3RecallLimit = 20

// MemoryRuntimeContext carries session-local identifiers for scope resolution.
type MemoryRuntimeContext struct {
	AgentID   string
	UserID    string
	TeamID    string
	Workspace string
}

// L3ScopeTarget is one L3 recall scope bucket.
type L3ScopeTarget struct {
	ScopeType string
	ScopeID   string
}

// MemoryRuntimePolicy is the resolved read/write policy for agent memory at runtime.
type MemoryRuntimePolicy struct {
	MasterEnabled bool

	InjectL1 bool
	RecallL2 bool
	InjectL3 bool
	InjectL4 bool

	WriteL3Facts    bool
	WriteL2Episode  bool
	WriteL4Graph    bool
	WriteConsolidate bool

	L2RecallMax           int
	L3RecallTopK          int
	L3MinScoreQuery       float64
	L3MinScorePassive     float64
	L3MaxPerRecallChars   int
	L3RecallScopes        []string
	L0L3MaxChunks         int
	L0L4MaxPaths          int
	L1FieldMaxChars       int
	L4PersonaMaxChars     int
	L4GraphInjectNeighbors bool
	L4GraphMaxNeighbors    int
	L4GraphMaxHops         int
	L4IdentityInject       bool
	L4StrategyInject       bool

	MemoryToolMaxResults int
	MemoryToolMinScore   float64

	L2RetentionDays      int
	L3DecayIntervalHours int
}

func (p MemoryRuntimePolicy) AnyInject() bool {
	return p.InjectL1 || p.RecallL2 || p.InjectL3 || p.InjectL4
}

func (p MemoryRuntimePolicy) AnyWrite() bool {
	return p.WriteConsolidate
}

// ResolveMemoryRuntimePolicy maps agent_runtime_settings + optional session context
// into symmetric read/write gates. Missing settings fail closed.
func ResolveMemoryRuntimePolicy(settings *AgentRuntimeSettings) MemoryRuntimePolicy {
	if settings == nil || !settings.MemoryEnabled {
		return MemoryRuntimePolicy{}
	}
	p := MemoryRuntimePolicy{
		MasterEnabled:          true,
		InjectL1:               settings.L1Enabled && settings.L0InjectL1,
		RecallL2:               settings.L2RecallEnabled,
		InjectL3:               settings.L3Enabled && settings.L0InjectL3,
		InjectL4:               settings.L4Enabled && settings.L0InjectL4,
		WriteL3Facts:           settings.L3Enabled,
		WriteL2Episode:         settings.L2EpisodeEnabled,
		WriteL4Graph:           settings.L4Enabled,
		L2RecallMax:            settings.L2RecallMax,
		L3RecallTopK:           settings.L3RecallTopK,
		L3MinScoreQuery:        settings.L3RecallMinScore,
		L3MinScorePassive:      0,
		L3MaxPerRecallChars:    settings.L3MaxPerRecallChars,
		L3RecallScopes:         parseMemoryScopeList(settings.L3RecallScopesJSON),
		L0L3MaxChunks:          settings.L0L3MaxChunks,
		L0L4MaxPaths:           settings.L0L4MaxPaths,
		L1FieldMaxChars:        settings.L1FieldMaxTokens * 4,
		L4PersonaMaxChars:      settings.EvoPersonaMaxChars,
		L4GraphInjectNeighbors: settings.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:    settings.L4GraphMaxNeighbors,
		L4GraphMaxHops:         settings.L4GraphMaxHops,
		L4IdentityInject:       settings.L4IdentityInject,
		L4StrategyInject:       settings.L4StrategyInject,
		MemoryToolMaxResults:   settings.MemoryMaxResults,
		MemoryToolMinScore:     settings.MemoryMinScore,
		L2RetentionDays:        settings.L2RetentionDays,
		L3DecayIntervalHours:   settings.L3DecayIntervalHours,
	}
	if p.L2RecallMax <= 0 {
		p.L2RecallMax = 3
	}
	if p.L3RecallTopK <= 0 {
		p.L3RecallTopK = 12
	}
	if p.L0L3MaxChunks > 0 && p.L3RecallTopK > p.L0L3MaxChunks {
		p.L3RecallTopK = p.L0L3MaxChunks
	}
	if p.L3RecallTopK > maxL3RecallLimit {
		p.L3RecallTopK = maxL3RecallLimit
	}
	if p.L3MaxPerRecallChars <= 0 {
		p.L3MaxPerRecallChars = 1500
	}
	if p.L1FieldMaxChars <= 0 {
		p.L1FieldMaxChars = 2048 * 4
	}
	if p.L0L4MaxPaths <= 0 {
		p.L0L4MaxPaths = 8
	}
	if p.L4PersonaMaxChars <= 0 {
		p.L4PersonaMaxChars = 1500
	}
	if p.MemoryToolMaxResults <= 0 {
		p.MemoryToolMaxResults = 6
	}
	if p.MemoryToolMinScore <= 0 {
		p.MemoryToolMinScore = 0.35
	}
	if p.L2RetentionDays <= 0 {
		p.L2RetentionDays = 90
	}
	if p.L3DecayIntervalHours <= 0 {
		p.L3DecayIntervalHours = 24
	}
	if len(p.L3RecallScopes) == 0 {
		p.L3RecallScopes = []string{"agent"}
	}
	p.WriteConsolidate = p.WriteL3Facts || p.WriteL2Episode || p.WriteL4Graph
	return p
}

func parseMemoryScopeList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var scopes []string
	if json.Unmarshal([]byte(raw), &scopes) != nil {
		return nil
	}
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// L3ScopeTargets resolves configured recall scopes for the current session context.
func L3ScopeTargets(rt MemoryRuntimeContext, scopes []string) []L3ScopeTarget {
	if len(scopes) == 0 {
		scopes = []string{"agent"}
	}
	seen := make(map[string]struct{})
	var out []L3ScopeTarget
	appendScope := func(scopeType, scopeID string) {
		scopeType = strings.TrimSpace(scopeType)
		scopeID = strings.TrimSpace(scopeID)
		if scopeType == "" {
			return
		}
		if scopeType != "global" && scopeID == "" {
			return
		}
		if scopeType == "global" && scopeID == "" {
			scopeID = "global"
		}
		key := scopeType + ":" + scopeID
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, L3ScopeTarget{ScopeType: scopeType, ScopeID: scopeID})
	}
	for _, scope := range scopes {
		switch strings.ToLower(strings.TrimSpace(scope)) {
		case "agent":
			appendScope("agent", rt.AgentID)
		case "user":
			appendScope("user", rt.UserID)
		case "team":
			appendScope("team", rt.TeamID)
		case "workspace":
			ws := strings.TrimSpace(rt.Workspace)
			if ws == "" {
				ws = rt.AgentID
			}
			appendScope("workspace", ws)
		case "global":
			appendScope("global", "global")
		}
	}
	if len(out) == 0 {
		appendScope("agent", rt.AgentID)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScopeType == out[j].ScopeType {
			return out[i].ScopeID < out[j].ScopeID
		}
		return out[i].ScopeType < out[j].ScopeType
	})
	return out
}

func EffectiveL3MinScore(p MemoryRuntimePolicy, query string) float64 {
	if strings.TrimSpace(query) != "" {
		return p.L3MinScoreQuery
	}
	return p.L3MinScorePassive
}
