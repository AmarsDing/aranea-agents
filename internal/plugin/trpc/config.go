package plugintrpc

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func parsePluginConfig(configJSON, defaultJSON string, dest any) {
	if dest == nil {
		return
	}
	merged := map[string]any{}
	if strings.TrimSpace(defaultJSON) != "" && defaultJSON != "{}" {
		_ = json.Unmarshal([]byte(defaultJSON), &merged)
	}
	if strings.TrimSpace(configJSON) != "" && configJSON != "{}" {
		var overlay map[string]any
		if err := json.Unmarshal([]byte(configJSON), &overlay); err == nil {
			for k, v := range overlay {
				merged[k] = v
			}
		}
	}
	if len(merged) == 0 {
		return
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, dest)
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func stringSliceField(m map[string]any, key string) []string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func boolField(m map[string]any, key string, def bool) bool {
	raw, ok := m[key]
	if !ok {
		return def
	}
	if b, ok := raw.(bool); ok {
		return b
	}
	return def
}

func intField(m map[string]any, key string, def int) int {
	raw, ok := m[key]
	if !ok {
		return def
	}
	switch v := raw.(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return def
	}
}

func stringFieldMap(m map[string]any, key string) string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return ""
	}
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

var (
	emailRE = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	phoneRE = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	secretRE = regexp.MustCompile(`(?i)(sk-[a-zA-Z0-9]{8,}|api[_-]?key\s*[:=]\s*\S+|Bearer\s+[a-zA-Z0-9._\-]+)`)
)

func redactText(s string, maskEmail, maskPhone, maskSecret bool) string {
	if s == "" {
		return s
	}
	if maskEmail {
		s = emailRE.ReplaceAllString(s, "[email redacted]")
	}
	if maskPhone {
		s = phoneRE.ReplaceAllString(s, "[phone redacted]")
	}
	if maskSecret {
		s = secretRE.ReplaceAllString(s, "[secret redacted]")
	}
	return s
}

func containsAny(s string, patterns []string) bool {
	lower := strings.ToLower(s)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func modelNameFromContext(ctx context.Context) string {
	if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil && inv.Model != nil {
		return strings.TrimSpace(inv.Model.Info().Name)
	}
	return ""
}

func sessionAgentKey(ctx context.Context, inv *trpcagent.Invocation) (sessionID, agentKey string) {
	if inv == nil {
		if i, ok := trpcagent.InvocationFromContext(ctx); ok && i != nil {
			inv = i
		}
	}
	if inv != nil {
		if inv.Session != nil {
			sessionID = inv.Session.ID
		}
		agentKey = inv.AgentName
	}
	return
}

func patchRequestModelHint(req *trpcmodel.Request, model string) {
	if req == nil || strings.TrimSpace(model) == "" {
		return
	}
	if req.ExtraFields == nil {
		req.ExtraFields = make(map[string]any)
	}
	req.ExtraFields["aranea_model_override"] = model
}

func toolInList(name string, list []string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, t := range list {
		if strings.ToLower(strings.TrimSpace(t)) == name {
			return true
		}
	}
	return false
}
