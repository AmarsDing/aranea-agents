package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

const (
	l4CueMinConfidence      = 0.3
	l4CueTentativeThreshold = 0.6
	// l4CueCandidateLimit bounds how many active entities are fetched per turn
	// for in-memory mention ranking. The personal KG is small by design
	// (L4.md: ≤ 5万 nodes/agent upper bound, realistically dozens), so a
	// bounded recent slice is enough to find mentioned entities while keeping
	// the per-turn read cheap.
	l4CueCandidateLimit = 64
)

// L4MemoryCue builds the L4 knowledge-graph cue. It also returns the IDs of
// the entities actually injected into the cue (post mention-ranking,
// confidence gate and maxPaths truncation) so the caller can trigger memory
// reconsolidation (design §15.7) for exactly the recalled set.
func L4MemoryCue(ctx context.Context, admin biz.L4EntityStore, ag biz.Agent, policy biz.MemoryRuntimePolicy, query string, lg loggateway.Logger) (string, []string) {
	if admin == nil || !policy.InjectL4 {
		return "", nil
	}
	maxPaths := policy.L0L4MaxPaths
	query = strings.TrimSpace(query)

	var parts []string
	// recalledIDs collects the IDs of entities actually injected into the cue.
	var recalledIDs []string
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

	// Entity selection: do NOT pass `query` as the SQL keyword filter. The
	// recall keyword is a full user sentence / intent-hint blob (up to 120
	// chars), while entity names are short tokens — `name LIKE '%<sentence>%'`
	// can never match, which silently starved the L4 cue to zero rows on every
	// turn. Fetch a bounded candidate slice instead and rank in Go: entities
	// whose name literally appears in the user message first (mention
	// relevance), then confidence desc, then store recency order. The
	// confidence gate below still applies to every candidate.
	rows, _, err := admin.ListEntityRows(ctx, "agent", ag.ID, "", "", "", "active", "", l4CueCandidateLimit, 0)
	if err != nil {
		lg.Warn("L4 memory query failed", loggateway.StepID("agent.memory_query_fail"), loggateway.Err(err))
	}
	if err == nil && len(rows) > 0 {
		rows = rankL4CueCandidates(rows, query, maxPaths)
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
			if id := strings.TrimSpace(fmt.Sprint(ent["id"])); id != "" {
				recalledIDs = append(recalledIDs, id)
			}
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
	return strings.TrimSpace(strings.Join(parts, "\n\n")), recalledIDs
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

// rankL4CueCandidates filters cue-ineligible entity rows and orders the rest
// for injection: entities whose name literally appears in the user query
// (mention relevance) rank first, then confidence desc; ties keep the store's
// recency order. Returns at most maxPaths rows (maxPaths<=0 → no limit beyond
// the candidate slice). Rows failing JSON parse, with empty names, or below
// l4CueMinConfidence are dropped BEFORE truncation so they never consume an
// injection slot.
func rankL4CueCandidates(rows [][]byte, query string, maxPaths int) [][]byte {
	type cand struct {
		raw       []byte
		nameLower string
		conf      float64
		mentioned bool
		idx       int
	}
	kw := strings.ToLower(strings.TrimSpace(query))
	cands := make([]cand, 0, len(rows))
	for i, raw := range rows {
		var ent map[string]any
		if json.Unmarshal(raw, &ent) != nil {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(ent["name"]))
		if name == "" {
			continue
		}
		conf := floatVal(ent["confidence"])
		if conf < l4CueMinConfidence {
			continue
		}
		nl := strings.ToLower(name)
		cands = append(cands, cand{
			raw:       raw,
			nameLower: nl,
			conf:      conf,
			mentioned: kw != "" && strings.Contains(kw, nl),
			idx:       i,
		})
	}
	sort.SliceStable(cands, func(a, b int) bool {
		if cands[a].mentioned != cands[b].mentioned {
			return cands[a].mentioned
		}
		if cands[a].conf != cands[b].conf {
			return cands[a].conf > cands[b].conf
		}
		return cands[a].idx < cands[b].idx
	})
	if maxPaths > 0 && len(cands) > maxPaths {
		cands = cands[:maxPaths]
	}
	out := make([][]byte, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.raw)
	}
	return out
}
