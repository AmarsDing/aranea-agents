package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// L4MemoryCue appends knowledge-graph context when L4 is enabled for the agent.
func L4MemoryCue(ctx context.Context, admin biz.SessionAdminStore, ag biz.Agent) string {
	if admin == nil || ag.Settings == nil || !ag.Settings.L4Enabled || !ag.Settings.L0InjectL4 {
		return ""
	}
	rows, _, err := admin.ListEntityRows(ctx, "agent", ag.ID, "", "", "", "active", "", 12, 0)
	if err != nil || len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L4 knowledge graph (session memory)\n")
	b.WriteString("The following entities and relations are retrieved from long-term memory. Use them when relevant; do not invent facts beyond this graph.\n")
	for i, raw := range rows {
		if i >= 8 {
			break
		}
		var ent map[string]any
		if json.Unmarshal(raw, &ent) != nil {
			continue
		}
		name := strings.TrimSpace(fmt.Sprint(ent["name"]))
		etype := strings.TrimSpace(fmt.Sprint(ent["entity_type"]))
		desc := strings.TrimSpace(fmt.Sprint(ent["description"]))
		if name == "" {
			continue
		}
		fmt.Fprintf(&b, "- [%s] %s", etype, name)
		if desc != "" && desc != "<nil>" {
			fmt.Fprintf(&b, ": %s", desc)
		}
		b.WriteByte('\n')
	}
	if ag.Settings.L4GraphInjectNeighbors && len(rows) > 0 {
		var first map[string]any
		if json.Unmarshal(rows[0], &first) == nil {
			if id := strings.TrimSpace(fmt.Sprint(first["id"])); id != "" {
				hops := int32(ag.Settings.L4GraphMaxHops)
				if hops <= 0 {
					hops = 1
				}
				maxN := int32(ag.Settings.L4GraphMaxNeighbors)
				if maxN <= 0 {
					maxN = 5
				}
				if nb, err := admin.NeighborhoodJSON(ctx, id, hops, maxN, ""); err == nil && len(nb) > 2 {
					b.WriteString("\nNeighborhood (JSON):\n")
					b.Write(nb)
					b.WriteByte('\n')
				}
			}
		}
	}
	return strings.TrimSpace(b.String())
}
