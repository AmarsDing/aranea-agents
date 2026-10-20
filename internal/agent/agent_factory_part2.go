package agent

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
)

// findSameDomainAgent 在同域 Agent 中查找使命高相似（cosine ≥ 0.85）者复用。
// DomainPath 为空或 embedder 为 nil 时跳过（仅 key 命中复用，不变量 3）。
func (f *AgentFactoryImpl) findSameDomainAgent(ctx context.Context, profile biz.TaskProfile) (string, bool) {
	dp := NormalizeDomainPath(profile.DomainPath)
	if dp == "" || f.embedder == nil {
		return "", false
	}
	res, err := f.agentReader.SearchAgents(ctx, biz.AgentListQuery{Status: "active", Limit: sameDomainReuseScanLimit})
	if err != nil || len(res.Items) == 0 {
		return "", false
	}
	cands := make([]biz.Agent, 0, len(res.Items))
	for _, ag := range res.Items {
		if biz.IsSystemAgentKey(ag.AgentKey) {
			continue
		}
		if DomainPathRelated(ag.DomainPath, dp) {
			cands = append(cands, ag)
		}
	}
	if len(cands) == 0 {
		return "", false
	}
	query := profile.Mission
	if query == "" {
		query = profile.TaskDescription
	}
	texts := make([]string, 0, len(cands)+1)
	texts = append(texts, query)
	for _, ag := range cands {
		m := ag.MissionStatement
		if m == "" {
			m = ag.AgentDescription
		}
		texts = append(texts, m)
	}
	vectors, err := f.embedder.Embed(ctx, texts)
	if err != nil || len(vectors) != len(texts) || len(vectors[0]) == 0 {
		if err != nil {
			f.lg.Warn("AgentFactory 同域复用 embedding 失败，跳过",
				loggateway.StepID("agent_factory.domain_reuse"),
				loggateway.Err(err),
			)
		}
		return "", false
	}
	bestIdx, bestSim := -1, 0.0
	for i := range cands {
		if sim := cosineSimilarity32(vectors[0], vectors[i+1]); sim > bestSim {
			bestSim, bestIdx = sim, i
		}
	}
	if bestIdx < 0 || bestSim < sameDomainReuseMinSim {
		return "", false
	}
	return cands[bestIdx].AgentKey, true
}

// keywordOverlapScore computes the fraction of tokens in `a` that also appear in `b`.
func keywordOverlapScore(a, b string) float64 {
	aTokens := tokenizeForSemantic(a)
	bTokens := tokenizeForSemantic(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0
	}
	bSet := make(map[string]bool, len(bTokens))
	for _, t := range bTokens {
		bSet[t] = true
	}
	overlap := 0
	for _, t := range aTokens {
		if bSet[t] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(aTokens))
}

// resolveBirthPosition picks an existing position under DepartmentID.
// Factory never creates company/department/position nodes (ORGFAST-05/12).
func (f *AgentFactoryImpl) resolveBirthPosition(ctx context.Context, profile biz.TaskProfile) (positionID, positionKey string) {
	if id := strings.TrimSpace(profile.PositionID); id != "" {
		return id, ""
	}
	deptID := strings.TrimSpace(profile.DepartmentID)
	if deptID == "" || f.org == nil {
		return "", ""
	}
	children, err := f.org.ListOrgNodesByParentID(ctx, deptID)
	if err != nil {
		f.lg.Warn("AgentFactory 查询部门岗位失败，新 Agent 暂不占岗",
			loggateway.StepID("agent_factory.position"),
			loggateway.Str("department_id", deptID),
			loggateway.Err(err),
		)
		return "", ""
	}
	var fallback biz.OrganizationNode
	for _, n := range children {
		if n.Level != "position" || strings.TrimSpace(n.DeletedAt) != "" {
			continue
		}
		if fallback.ID == "" {
			fallback = n
		}
		label := strings.ToLower(n.Name + " " + n.Key)
		if strings.Contains(label, "其他") || strings.Contains(label, "other") || strings.Contains(label, "general") {
			return n.ID, n.Key
		}
	}
	return fallback.ID, fallback.Key
}
