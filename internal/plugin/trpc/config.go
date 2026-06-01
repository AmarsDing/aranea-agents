package plugintrpc

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

func parsePluginConfig(configJSON, defaultJSON string, dest any, lg loggateway.Logger) {
	if dest == nil {
		return
	}
	merged := map[string]any{}
	if strings.TrimSpace(defaultJSON) != "" && defaultJSON != "{}" {
		if err := json.Unmarshal([]byte(defaultJSON), &merged); err != nil {
			lg.Warn("解析 plugin default config 失败", loggateway.StepID("plugin.trpc.config"), loggateway.Err(err))
		}
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
	if err := json.Unmarshal(b, dest); err != nil {
		lg.Warn("解析 plugin merged config 失败", loggateway.StepID("plugin.trpc.config"), loggateway.Err(err))
	}
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

type customPattern struct {
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
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

func agentKeyFromInvocation(inv *trpcagent.Invocation) string {
	if inv == nil {
		return ""
	}
	return strings.TrimSpace(inv.AgentName)
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
