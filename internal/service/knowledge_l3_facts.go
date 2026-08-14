package service

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// L3AgentFactLister 把 L3 JSON 行投影为知识库写回/G1 投影事实。
type L3AgentFactLister struct {
	r biz.L3FactReader
}

// NewL3AgentFactLister r 为 nil 时 List 返回空切片。
func NewL3AgentFactLister(r biz.L3FactReader) *L3AgentFactLister {
	return &L3AgentFactLister{r: r}
}

func (a *L3AgentFactLister) ListAgentFacts(ctx context.Context, agentID string, limit int) ([]bizknowledge.WriteBackFact, error) {
	if a == nil || a.r == nil || strings.TrimSpace(agentID) == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 80
	}
	rows, _, _, _, err := a.r.ListFactRows(ctx, "", "", "", "active", "", agentID, int32(limit), 0)
	if err != nil {
		return nil, err
	}
	out := make([]bizknowledge.WriteBackFact, 0, len(rows))
	for _, raw := range rows {
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		stmt, _ := m["statement"].(string)
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		kind, _ := m["fact_kind"].(string)
		id, _ := m["id"].(string)
		conf := 0.0
		switch v := m["confidence"].(type) {
		case float64:
			conf = v
		case float32:
			conf = float64(v)
		}
		src, _ := m["source_kind"].(string)
		out = append(out, bizknowledge.WriteBackFact{
			FactID:     id,
			Statement:  stmt,
			FactKind:   kind,
			Confidence: conf,
			SourceKind: src,
		})
	}
	return out, nil
}
