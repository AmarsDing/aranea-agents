package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	l4CueMinConfidence      = 0.3
	l4CueTentativeThreshold = 0.6
)

func L4MemoryCue(ctx context.Context, admin biz.SessionAdminStore, ag biz.Agent, policy biz.MemoryRuntimePolicy, query string, lg loggateway.Logger) string {
	if admin == nil || !policy.InjectL4 {
		return ""
	}
	maxPaths := policy.L0L4MaxPaths
	query = strings.TrimSpace(query)

	var parts []string
	if policy.L4IdentityInject {
		if raw, err := admin.AgentIdentityJSON(ctx, ag.ID); err == nil && len(raw) > 2 {
			if block := formatL4JSONBlock("L4 agent identity", raw, policy.L4PersonaMaxChars); block != "" {
				parts = append(parts, block)
			}
		}
	}
	if policy.L4StrategyInject {
		if raw, err := admin.AgentStrategyJSON(ctx, ag.ID); err == nil && len(raw) > 2 {
			if block := formatL4JSONBlock("L4 agent strategy", raw, policy.L4PersonaMaxChars); block != "" {
				parts = append(parts, block)
			}
		}
	}

	rows, _, err := admin.ListEntityRows(ctx, "agent", ag.ID, "", "", "", "active", query, int32(maxPaths+4), 0)
	if err != nil {
		lg.Warn("L4 memory query failed", loggateway.StepID("agent.memory_query_fail"), loggateway.Err(err))
	}
	if err == nil && len(rows) > 0 {
		var b strings.Builder
		b.WriteString("## L4 knowledge graph (session memory)\n")
		b.WriteString("The following entities and relations are retrieved from long-term memory. Use them when relevant; do not invent facts beyond this graph.\n")
		for i, raw := range rows {
			if i >= maxPaths {
				break
			}
			var ent map[string]any
			if json.Unmarshal(raw, &ent) != nil {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(ent["name"]))
			etype := strings.TrimSpace(fmt.Sprint(ent["entity_type"]))
			desc := strings.TrimSpace(fmt.Sprint(ent["description"]))
			confidence := floatVal(ent["confidence"])
			if name == "" {
				continue
			}
			if confidence < l4CueMinConfidence {
				continue
			}
			fmt.Fprintf(&b, "- [%s] %s", etype, name)
			if desc != "" && desc != "<nil>" {
				fmt.Fprintf(&b, ": %s", desc)
			}
			if confidence < l4CueTentativeThreshold {
				b.WriteString(" (tentative — may be outdated, verify if uncertain)")
			}
			b.WriteByte('\n')
		}
		if policy.L4GraphInjectNeighbors && len(rows) > 0 {
			var first map[string]any
			if json.Unmarshal(rows[0], &first) == nil {
				if id := strings.TrimSpace(fmt.Sprint(first["id"])); id != "" {
					hops := int32(policy.L4GraphMaxHops)
					if hops <= 0 {
						hops = 1
					}
					maxN := int32(policy.L4GraphMaxNeighbors)
					if maxN <= 0 {
						maxN = 5
					}
					if nb, err := admin.NeighborhoodJSON(ctx, id, hops, maxN, ""); err == nil && len(nb) > 2 {
						nbText := truncateText(string(nb), policy.L4PersonaMaxChars)
						b.WriteString("\nNeighborhood (JSON):\n")
						b.WriteString(nbText)
						b.WriteByte('\n')
					}
				}
			}
		}
		if graph := strings.TrimSpace(b.String()); graph != "" {
			parts = append(parts, graph)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func formatL4JSONBlock(title string, raw []byte, maxChars int) string {
	body := strings.TrimSpace(string(raw))
	if body == "" || body == "{}" || body == "null" {
		return ""
	}
	body = truncateText(body, maxChars)
	return fmt.Sprintf("## %s\n```json\n%s\n```", title, body)
}

func safeTruncate(s string, maxChars int) string {
	if maxChars <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars]) + "…"
}

func truncateText(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	return safeTruncate(s, maxChars)
}

func floatVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
