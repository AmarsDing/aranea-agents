package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
)

// StaffingLLMTimeout is the single staffing consult budget (M78 ORGFAST-21).
// Timeout fails closed (roster miss); Factory is opt-in only.
const StaffingLLMTimeout = 4 * time.Second

func observeOrgFastDeptLead(layer string) {
	switch strings.TrimSpace(layer) {
	case "domain_recipe", "mission", "roster":
		metrics.OrgFastDeptLeadTotal.WithLabelValues("skipped_high_confidence").Inc()
	}
}

func (impl *agentAllocatorImpl) resolveStaffingAdvisor() biz.StaffingAdvisor {
	if impl == nil {
		return nil
	}
	if impl.staffingAdvisor != nil {
		return impl.staffingAdvisor
	}
	if impl.catalog != nil && impl.httpClient != nil {
		return llmStaffingAdvisor{impl: impl}
	}
	return nil
}

func (impl *agentAllocatorImpl) staffingWait() time.Duration {
	if impl != nil && impl.staffingTimeout > 0 {
		return impl.staffingTimeout
	}
	return StaffingLLMTimeout
}

// tryStaffing asks the department lead once when L0–L3 missed. It never
// re-decomposes the user's original task. Timeout / Factory / nil advisor
// all return ok=false so the caller fail-closes (no hot-path Factory).
func (impl *agentAllocatorImpl) tryStaffing(
	ctx context.Context,
	subTask biz.SubTask,
	pool []biz.AgentCapability,
	prune OrgPruneResult,
	traceID string,
) (biz.TaskAllocation, bool) {
	advisor := impl.resolveStaffingAdvisor()
	if advisor == nil {
		return biz.TaskAllocation{}, false
	}
	if prune.FallbackAll && len(prune.DepartmentIDs) == 0 {
		return biz.TaskAllocation{}, false
	}
	candidates := make([]string, 0, len(pool))
	cards := make([]string, 0, len(pool))
	allowed := make(map[string]biz.AgentCapability, len(pool))
	for _, cap := range pool {
		if !cap.IsHeuristicAssignable() {
			continue
		}
		candidates = append(candidates, cap.AgentKey)
		allowed[cap.AgentKey] = cap
		cards = append(cards, staffingCard(cap))
	}
	if len(candidates) == 0 {
		return biz.TaskAllocation{}, false
	}
	deptID := ""
	if len(prune.DepartmentIDs) > 0 {
		deptID = prune.DepartmentIDs[0]
	}
	askCtx, cancel := context.WithTimeout(ctx, impl.staffingWait())
	defer cancel()
	metrics.OrgFastDeptLeadTotal.WithLabelValues("staffing_asked").Inc()
	reply, err := advisor.Suggest(askCtx, biz.StaffingAsk{
		DepartmentID:   deptID,
		DomainPath:     subTask.DomainPath,
		SubTaskName:    strings.TrimSpace(subTask.Name),
		CandidateKeys:  candidates,
		CandidateCards: cards,
	})
	if err != nil {
		if askCtx.Err() != nil {
			metrics.OrgFastDeptLeadTotal.WithLabelValues("staffing_timeout").Inc()
			impl.lg.Warn("主管 staffing 超时，fail-closed",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("department_id", deptID),
				loggateway.Err(err),
			)
			return biz.TaskAllocation{}, false
		}
		impl.lg.Warn("主管 staffing 失败，fail-closed",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("department_id", deptID),
			loggateway.Err(err),
		)
		return biz.TaskAllocation{}, false
	}
	if reply.UseFactory || len(reply.AgentKeys) == 0 {
		metrics.OrgFastDeptLeadTotal.WithLabelValues("staffing_factory").Inc()
		return biz.TaskAllocation{}, false
	}
	key := strings.TrimSpace(reply.AgentKeys[0])
	cap, ok := allowed[key]
	if !ok {
		metrics.OrgFastDeptLeadTotal.WithLabelValues("staffing_factory").Inc()
		impl.lg.Warn("主管 staffing 返回非候选 Agent，忽略",
			loggateway.StepID(biz.SpiritStepAllocatorMatch),
			loggateway.Str("trace_id", traceID),
			loggateway.Str("returned_key", key),
		)
		return biz.TaskAllocation{}, false
	}
	metrics.OrgFastDeptLeadTotal.WithLabelValues("staffing_adopted").Inc()
	return biz.TaskAllocation{
		SubTaskID:    subTask.ID,
		SubTaskName:  subTask.Name,
		AssignedType: "agent",
		AssignedKey:  cap.AgentKey,
		AssignedName: cap.DisplayName,
		MatchScore:   0,
		MatchLayer:   "staffing",
		MatchReason:  "部门主管 staffing 建议（未二次分解）",
	}, true
}

// llmStaffingAdvisor asks the planner model to pick from CandidateKeys only.
type llmStaffingAdvisor struct {
	impl *agentAllocatorImpl
}

func (a llmStaffingAdvisor) Suggest(ctx context.Context, in biz.StaffingAsk) (biz.StaffingReply, error) {
	if a.impl == nil || a.impl.catalog == nil || a.impl.httpClient == nil {
		return biz.StaffingReply{UseFactory: true}, nil
	}
	prompt := "You are a department lead doing staffing, not planning.\n" +
		"Pick ONE agent_key from candidate_keys, or set factory=true.\n" +
		"Do not decompose the original user task. Do not invent keys.\n" +
		"Reply JSON only: {\"agent_key\":\"...\",\"factory\":false}"
	user := "department_id=" + in.DepartmentID +
		"\ndomain_path=" + in.DomainPath +
		"\nsubtask=" + in.SubTaskName +
		"\ncandidate_keys=" + strings.Join(in.CandidateKeys, ",")
	if len(in.CandidateCards) > 0 {
		user += "\ncandidates=\n" + strings.Join(in.CandidateCards, "\n")
	}
	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if a.impl.plannerSetting != nil {
		if s, err := a.impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, a.impl.catalog, a.impl.lg, biz.SpiritStepAllocatorMatch, "StaffingAdvisor")
	if provider == "" || model == "" {
		return biz.StaffingReply{UseFactory: true}, nil
	}
	row, err := a.impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return biz.StaffingReply{}, err
	}
	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	text, _, _, _, err := CallOpenAICompatChat(ctx, a.impl.httpClient, cfg, model, []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: user},
	})
	if err != nil {
		return biz.StaffingReply{}, err
	}
	return parseStaffingReply(text, in.CandidateKeys), nil
}

func staffingCard(cap biz.AgentCapability) string {
	mission := strings.TrimSpace(cap.Mission)
	if mission == "" {
		mission = strings.TrimSpace(cap.Description)
	}
	if runes := []rune(mission); len(runes) > 80 {
		mission = string(runes[:80])
	}
	return cap.AgentKey + "|" + strings.TrimSpace(cap.DisplayName) + "|" +
		strings.TrimSpace(cap.DomainPath) + "|" + mission
}

func parseStaffingReply(text string, candidates []string) biz.StaffingReply {
	text = strings.TrimSpace(text)
	var raw struct {
		AgentKey string `json:"agent_key"`
		Factory  bool   `json:"factory"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &raw)
		}
	}
	if raw.Factory {
		return biz.StaffingReply{UseFactory: true}
	}
	key := strings.TrimSpace(raw.AgentKey)
	for _, c := range candidates {
		if c == key {
			return biz.StaffingReply{AgentKeys: []string{key}}
		}
	}
	return biz.StaffingReply{UseFactory: true}
}
