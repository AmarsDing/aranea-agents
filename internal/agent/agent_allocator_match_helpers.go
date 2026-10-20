package agent

import (
	"aranea-agents/internal/biz"

	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"

	"fmt"

	"strings"

	"time"
)

// selectAdditionalMembers picks `count` additional agent keys from the
// capability pool, excluding the primary key and non-assignable roles.
// Preference: same department + complementary roles, then same department,
// then cross-department (returned separately for borrow marking).
func (impl *agentAllocatorImpl) selectAdditionalMembers(primaryKey string, capabilities []biz.AgentCapability, count int) (members []string, crossDept []string) {
	return pickComplementaryMembers(primaryKey, capabilities, count)
}

// dagAdditionalMemberCount is how many extra specialists a dag team should
// add besides the lead. "三人/三位专家" means one team of three, not three
// teams — ask for two complements. "三个团队" stays at one complement.
func dagAdditionalMemberCount(userMessage string) int {
	msg := strings.TrimSpace(userMessage)
	if msg == "" {
		return 1
	}
	if strings.Contains(msg, "三个团队") || strings.Contains(msg, "三支团队") || strings.Contains(msg, "3个团队") {
		return 1
	}
	if strings.Contains(msg, "三人") || strings.Contains(msg, "三位") || strings.Contains(msg, "3人") {
		return 2
	}
	return 1
}

func pickComplementaryMembers(primaryKey string, capabilities []biz.AgentCapability, count int) (members []string, crossDept []string) {
	if count <= 0 || len(capabilities) == 0 {
		return nil, nil
	}
	var primary *biz.AgentCapability
	for i := range capabilities {
		if capabilities[i].AgentKey == primaryKey {
			primary = &capabilities[i]
			break
		}
	}
	primaryDept := ""
	var primaryRoles []string
	if primary != nil {
		primaryDept = primary.DepartmentID
		primaryRoles = primary.Roles
	}

	used := map[string]struct{}{primaryKey: {}}
	add := func(cap biz.AgentCapability) bool {
		if cap.AgentKey == "" {
			return false
		}
		if _, ok := used[cap.AgentKey]; ok {
			return false
		}
		if !cap.IsHeuristicAssignable() {
			return false
		}
		used[cap.AgentKey] = struct{}{}
		members = append(members, cap.AgentKey)
		if primaryDept != "" && cap.DepartmentID != "" && cap.DepartmentID != primaryDept {
			crossDept = append(crossDept, cap.AgentKey)
		}
		return len(members) >= count
	}

	passes := []func(biz.AgentCapability) bool{
		func(cap biz.AgentCapability) bool {
			return primaryDept != "" && cap.DepartmentID == primaryDept && hasComplementaryRole(primaryRoles, cap.Roles)
		},
		func(cap biz.AgentCapability) bool {
			return primaryDept != "" && cap.DepartmentID == primaryDept
		},
		func(cap biz.AgentCapability) bool {
			return hasComplementaryRole(primaryRoles, cap.Roles)
		},
		func(biz.AgentCapability) bool { return true },
	}
	for _, pred := range passes {
		for _, cap := range capabilities {
			if !pred(cap) {
				continue
			}
			if add(cap) {
				return members, crossDept
			}
		}
	}
	return members, crossDept
}

func hasComplementaryRole(lead, cand []string) bool {
	if len(lead) == 0 || len(cand) == 0 {
		return true
	}
	leadSet := make(map[string]struct{}, len(lead))
	for _, r := range lead {
		if t := strings.TrimSpace(r); t != "" {
			leadSet[t] = struct{}{}
		}
	}
	if len(leadSet) == 0 {
		return true
	}
	for _, r := range cand {
		if _, ok := leadSet[strings.TrimSpace(r)]; !ok {
			return true
		}
	}
	return false
}

func classifyCrossDeptMembers(primaryKey string, memberKeys []string, capabilities []biz.AgentCapability) (homeDept string, crossDept []string) {
	byKey := make(map[string]biz.AgentCapability, len(capabilities))
	for _, cap := range capabilities {
		byKey[cap.AgentKey] = cap
	}
	if p, ok := byKey[primaryKey]; ok {
		homeDept = p.DepartmentID
	}
	if homeDept == "" {
		return "", nil
	}
	for _, k := range memberKeys {
		cap, ok := byKey[k]
		if !ok || cap.DepartmentID == "" || cap.DepartmentID == homeDept {
			continue
		}
		crossDept = append(crossDept, k)
	}
	return homeDept, crossDept
}

// llmColdStart performs Layer 3 matching: use LLM to select the best agent.
func (impl *agentAllocatorImpl) llmColdStart(ctx context.Context, subTask biz.SubTask, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	capabilities = capLLMColdStart(filterHeuristicAssignable(capabilities))
	if len(capabilities) == 0 {
		return "", nil
	}

	prompt := buildAllocatorColdStartPrompt(subTask, capabilities)

	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepAllocatorMatch, "AgentAllocator")
	if provider == "" || model == "" {
		return "", apierror.Internal(apierror.DomainSpirit, "no provider/model configured for agent allocation (set planner_model_mode in system settings or add enabled models in catalog)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	level := biz.ComplexitySimple
	if subTask.EstimatedComplexity >= 0.6 {
		level = biz.ComplexityComplex
	} else if subTask.EstimatedComplexity >= 0.3 {
		level = biz.ComplexityModerate
	}
	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, level); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this subtask:\n\nName: %s\nDescription: %s\nRequired Capabilities: %v", subTask.Name, subTask.Description, subTask.RequiredCapabilities)},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.AllocateLLMTimeout)
	defer cancel()

	callStart := time.Now()
	text, _, promptTok, completionTok, cachedTok, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)

	recordSpiritAuxUsage(ctx, impl.auxUsage, impl.lg, biz.SpiritStepAllocatorMatch, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxAllocatorMatch,
		RunID:         subTask.ID,
		Provider:      provider,
		Model:         model,
		Status:        spiritAuxCallStatus(err),
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		CachedTok:     cachedTok,
		UsageSource:   biz.UsageSourceResponse,
		Effort:        cfg.ThinkingEffort,
		Latency:       time.Since(callStart),
		ErrMsg:        spiritAuxErrMsg(err),
		MetadataJSON:  spiritAuxMeta(ctx, "spirit_allocator_match"),
	})
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	agentKey := parseAllocatorColdStartResponse(text)

	if agentKey != "" {
		found := false
		for _, cap := range capabilities {
			if cap.AgentKey == agentKey {
				found = true
				break
			}
		}
		if !found {
			impl.lg.Warn("LLM cold-start returned unknown agent_key, falling back",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("returned_key", agentKey),
			)
			return "", nil
		}
	}
	return agentKey, nil
}

// llmColdStartForPlan uses LLM to select an agent for a whole plan (no subtasks).
func (impl *agentAllocatorImpl) llmColdStartForPlan(ctx context.Context, taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability, traceID string) (string, error) {
	if impl.catalog == nil || impl.httpClient == nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM catalog or HTTP client not configured")
	}

	capabilities = capLLMColdStart(filterHeuristicAssignable(capabilities))
	if len(capabilities) == 0 {
		return "", nil
	}

	prompt := buildAllocatorColdStartPromptForPlan(taskPlan, capabilities)

	setting := biz.PlannerModelSetting{Mode: biz.PlannerModelModeInherit}
	if impl.plannerSetting != nil {
		if s, err := impl.plannerSetting.GetPlannerModel(ctx); err == nil {
			setting = s
		}
	}
	sessionProvider, sessionModel := biz.PlannerSessionModelFromCtx(ctx)
	provider, model := ResolvePlannerModel(ctx, setting, sessionProvider, sessionModel, impl.catalog, impl.lg, biz.SpiritStepAllocatorMatch, "AgentAllocator")
	if provider == "" || model == "" {
		return "", apierror.Internal(apierror.DomainSpirit, "no provider/model configured for agent allocation (set planner_model_mode in system settings or add enabled models in catalog)")
	}

	row, err := impl.catalog.GetByProviderAndModel(ctx, provider, model)
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "get provider config").WithCause(err)
	}

	var cfg ProviderAPIConfig
	MergeProviderConfigJSON(row.ConfigJSON, &cfg)
	ApplyThinkingCapability(&cfg, row.CapabilitiesExplicit, row.Capabilities.Thinking)

	if eff := biz.ResolveThinkingEffort(cfg.ThinkingEffort, taskPlan.ComplexityLevel); eff != "" {
		cfg.ThinkingEffort = eff
	}

	msgs := []OpenAICompatMessage{
		{Role: "system", Content: prompt},
		{Role: "user", Content: fmt.Sprintf("Select the best agent for this task:\n\n%s", taskPlan.UserMessage)},
	}

	callCtx, cancel := context.WithTimeout(ctx, tools.AllocateLLMTimeout)
	defer cancel()

	callStart := time.Now()
	text, _, promptTok, completionTok, cachedTok, err := CallOpenAICompatChat(callCtx, impl.httpClient, cfg, model, msgs)

	recordSpiritAuxUsage(ctx, impl.auxUsage, impl.lg, biz.SpiritStepAllocatorMatch, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxAllocatorMatch,
		RunID:         traceID,
		Provider:      provider,
		Model:         model,
		Status:        spiritAuxCallStatus(err),
		PromptTok:     promptTok,
		CompletionTok: completionTok,
		CachedTok:     cachedTok,
		UsageSource:   biz.UsageSourceResponse,
		Effort:        cfg.ThinkingEffort,
		Latency:       time.Since(callStart),
		ErrMsg:        spiritAuxErrMsg(err),
		MetadataJSON:  spiritAuxMeta(ctx, "spirit_allocator_match"),
	})
	if err != nil {
		return "", apierror.Internal(apierror.DomainSpirit, "LLM call failed").WithCause(err)
	}

	agentKey := parseAllocatorColdStartResponse(text)

	if agentKey != "" {
		found := false
		for _, cap := range capabilities {
			if cap.AgentKey == agentKey {
				found = true
				break
			}
		}
		if !found {
			impl.lg.Warn("LLM cold-start (plan) returned unknown agent_key, falling back",
				loggateway.StepID(biz.SpiritStepAllocatorMatch),
				loggateway.Str("trace_id", traceID),
				loggateway.Str("returned_key", agentKey),
			)
			return "", nil
		}
	}
	return agentKey, nil
}

// computeOverlapRatio computes the ratio of overlap between required and available.
func computeOverlapRatio(required []string, available []string) float64 {
	if len(required) == 0 {
		return 0
	}
	availSet := make(map[string]bool, len(available))
	for _, a := range available {
		availSet[strings.ToLower(strings.TrimSpace(a))] = true
	}
	overlap := 0
	for _, r := range required {
		if availSet[strings.ToLower(strings.TrimSpace(r))] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(required))
}

// extractCapabilityHints extracts capability hints from a user message.
func extractCapabilityHints(userMessage string) []string {

	keywords := []string{
		"go-backend", "go-kratos", "vue3-frontend", "quasar-ui",
		"devops", "database", "architecture", "testing",
		"security", "research", "documentation", "api-design",
	}
	var hints []string
	msgLower := strings.ToLower(userMessage)
	for _, kw := range keywords {
		if strings.Contains(msgLower, strings.ReplaceAll(kw, "-", " ")) ||
			strings.Contains(msgLower, kw) {
			hints = append(hints, kw)
		}
	}
	if len(hints) == 0 {
		hints = append(hints, "general")
	}
	return hints
}

// buildAllocatorColdStartPrompt creates the system prompt for LLM-based agent selection.
func buildAllocatorColdStartPrompt(subTask biz.SubTask, capabilities []biz.AgentCapability) string {
	agentList := formatCapabilitiesForPrompt(capabilities)

	return `You are an agent allocation specialist. Select the best agent for a given subtask.

Rules:
- Output ONLY the agent_key of the best matching agent, no markdown, no explanation
- Consider the subtask's required capabilities and description
- If no agent is a good fit, output "none"

Available agents:
` + agentList
}

// buildAllocatorColdStartPromptForPlan creates the system prompt for LLM-based agent selection for a whole plan.
func buildAllocatorColdStartPromptForPlan(taskPlan *biz.TaskPlan, capabilities []biz.AgentCapability) string {
	agentList := formatCapabilitiesForPrompt(capabilities)

	return `You are an agent allocation specialist. Select the best agent for a given task.

Rules:
- Output ONLY the agent_key of the best matching agent, no markdown, no explanation
- Consider the task description and context
- If no agent is a good fit, output "none"

Available agents:
` + agentList
}

// formatCapabilitiesForPrompt formats agent capabilities for LLM prompt.
func formatCapabilitiesForPrompt(capabilities []biz.AgentCapability) string {
	var lines []string
	for _, cap := range capabilities {
		line := fmt.Sprintf("- agent_key: %s, display_name: %s, roles: %v, domain_path: %s, mission: %s, description: %s",
			cap.AgentKey, cap.DisplayName, cap.Roles, cap.DomainPath, cap.Mission, cap.Description)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// parseAllocatorColdStartResponse extracts the agent_key from LLM response.
func parseAllocatorColdStartResponse(text string) string {
	text = strings.TrimSpace(text)

	if m := fenceRE.FindStringSubmatch(text); len(m) > 1 {
		text = strings.TrimSpace(m[1])
	}
	// The response should be just the agent_key
	// Try to parse as JSON first
	var parsed struct {
		AgentKey string `json:"agent_key"`
	}
	if json.Unmarshal([]byte(text), &parsed) == nil && parsed.AgentKey != "" {
		return parsed.AgentKey
	}

	text = strings.TrimPrefix(text, "- agent_key:")
	text = strings.TrimSpace(text)
	if text == "none" || text == "" {
		return ""
	}
	return text
}
