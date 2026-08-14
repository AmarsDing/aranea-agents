package knowledge

import (
	"context"
	"sort"
	"strings"
)

// SP7 G8 知识健康度 + G7 专家定位（US-42 / US-43）。
// 只读聚合：图谱边密度 / 孤立率 / dangling / 写回日记新鲜度；专家从写回 provenance 聚合。

// CollectionHealth 单库知识图健康快照。
type CollectionHealth struct {
	DocumentCount   int
	EdgeCount       int
	ExplicitEdges   int
	IsolatedCount   int
	OrphanRate      float64 // IsolatedCount / max(DocumentCount,1)
	LinkDensity     float64 // EdgeCount / max(DocumentCount,1)
	DanglingCount   int
	WriteBackNotes  int
	WriteBackLatest string // 最新 inbox/writeback-YYYY-MM-DD.md 的 rel_path
}

// KnowledgeExpert 「谁懂这个」聚合行：写回 provenance 中的 agent/user。
type KnowledgeExpert struct {
	AgentID   string
	UserID    string
	FactCount int
	LastKind  string
}

// CollectionHealthSnapshot 计算库级健康度（读图谱 + dangling + 文档路径）。
func (u *Usecase) CollectionHealthSnapshot(ctx context.Context, collectionID string) (CollectionHealth, error) {
	if err := u.requireRepo(); err != nil {
		return CollectionHealth{}, err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return CollectionHealth{}, ErrCollectionIDRequired
	}
	g, err := u.ListCollectionGraph(ctx, collectionID, nil, "")
	if err != nil {
		return CollectionHealth{}, err
	}
	h := CollectionHealth{
		DocumentCount: len(g.Nodes),
		EdgeCount:     len(g.Edges),
	}
	isolated := 0
	for _, n := range g.Nodes {
		if n.Degree == 0 {
			isolated++
		}
	}
	h.IsolatedCount = isolated
	if h.DocumentCount > 0 {
		h.OrphanRate = float64(isolated) / float64(h.DocumentCount)
		h.LinkDensity = float64(h.EdgeCount) / float64(h.DocumentCount)
	}
	for _, e := range g.Edges {
		if e.Type == LinkTypeExplicit {
			h.ExplicitEdges++
		}
	}
	dangling, err := u.ListDanglingLinks(ctx, collectionID)
	if err != nil {
		return CollectionHealth{}, err
	}
	h.DanglingCount = len(dangling)

	docs, _, err := u.documents.ListDocuments(ctx, collectionID, maxGraphDocs, 0)
	if err != nil {
		return CollectionHealth{}, err
	}
	latest := ""
	for _, d := range docs {
		rel := strings.TrimSpace(d.RelPath)
		if !strings.HasPrefix(rel, "inbox/writeback-") || !strings.HasSuffix(rel, ".md") {
			continue
		}
		if strings.Contains(rel, "pending") {
			continue
		}
		h.WriteBackNotes++
		if rel > latest {
			latest = rel
		}
	}
	h.WriteBackLatest = latest
	return h, nil
}

// ListCollectionExperts 从写回日记与 agent 投影文档聚合 provenance。
func (u *Usecase) ListCollectionExperts(ctx context.Context, collectionID string) ([]KnowledgeExpert, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	collectionID = strings.TrimSpace(collectionID)
	if collectionID == "" {
		return nil, ErrCollectionIDRequired
	}
	docs, _, err := u.documents.ListDocuments(ctx, collectionID, maxGraphDocs, 0)
	if err != nil {
		return nil, err
	}
	type agg struct {
		agent, user, kind string
		count             int
	}
	byKey := map[string]*agg{}
	for _, summary := range docs {
		rel := strings.TrimSpace(summary.RelPath)
		if !isExpertSourcePath(rel) {
			continue
		}
		full, gerr := u.documents.GetDocument(ctx, summary.ID)
		if gerr != nil {
			continue
		}
		for _, hit := range ParseWriteBackExperts(full.ContentText) {
			key := hit.AgentID + "\x00" + hit.UserID
			a := byKey[key]
			if a == nil {
				a = &agg{agent: hit.AgentID, user: hit.UserID}
				byKey[key] = a
			}
			a.count += hit.FactCount
			if hit.LastKind != "" {
				a.kind = hit.LastKind
			}
		}
	}
	out := make([]KnowledgeExpert, 0, len(byKey))
	for _, a := range byKey {
		out = append(out, KnowledgeExpert{
			AgentID:   a.agent,
			UserID:    a.user,
			FactCount: a.count,
			LastKind:  a.kind,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FactCount != out[j].FactCount {
			return out[i].FactCount > out[j].FactCount
		}
		if out[i].AgentID != out[j].AgentID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].UserID < out[j].UserID
	})
	return out, nil
}

func isExpertSourcePath(rel string) bool {
	if strings.HasPrefix(rel, "inbox/writeback-") && strings.HasSuffix(rel, ".md") {
		return true
	}
	return strings.HasPrefix(rel, "agents/") && strings.HasSuffix(rel, ".md")
}

// ParseWriteBackExperts 从 provenance Markdown 聚合 agent/user（纯函数）。
func ParseWriteBackExperts(body string) []KnowledgeExpert {
	if strings.TrimSpace(body) == "" {
		return nil
	}
	type acc struct {
		count int
		kind  string
	}
	agents := map[string]*acc{}
	users := map[string]*acc{}
	curAgent, curUser, curKind := "", "", ""
	flush := func() {
		if curAgent != "" {
			a := agents[curAgent]
			if a == nil {
				a = &acc{}
				agents[curAgent] = a
			}
			a.count++
			if curKind != "" {
				a.kind = curKind
			}
		}
		if curUser != "" && curAgent == "" {
			u := users[curUser]
			if u == nil {
				u = &acc{}
				users[curUser] = u
			}
			u.count++
			if curKind != "" {
				u.kind = curKind
			}
		}
		curAgent, curUser, curKind = "", "", ""
	}
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "## ") {
			flush()
			if k := strings.TrimSpace(strings.TrimPrefix(trim, "## ")); k != "" && !strings.HasPrefix(k, "pending:") {
				curKind = k
			}
			continue
		}
		if !strings.HasPrefix(trim, "- ") {
			continue
		}
		key, val := splitPendingField(trim[2:])
		switch key {
		case "agent_id":
			curAgent = val
		case "user_id":
			curUser = val
		case "kind":
			if val != "" {
				curKind = val
			}
		}
	}
	flush()
	out := make([]KnowledgeExpert, 0, len(agents)+len(users))
	for id, a := range agents {
		out = append(out, KnowledgeExpert{AgentID: id, FactCount: a.count, LastKind: a.kind})
	}
	for id, a := range users {
		out = append(out, KnowledgeExpert{UserID: id, FactCount: a.count, LastKind: a.kind})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FactCount != out[j].FactCount {
			return out[i].FactCount > out[j].FactCount
		}
		if out[i].AgentID != out[j].AgentID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].UserID < out[j].UserID
	})
	return out
}
