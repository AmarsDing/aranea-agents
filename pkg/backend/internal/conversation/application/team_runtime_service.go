package application

import (
	mem "arenea/backend/internal/memory/domain"

	"arenea/backend/internal/kernel/errs"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/kernel/runctx"
)

type teamDefinition struct {
	Version          int          `json:"version"`
	Description      string       `json:"description"`
	Mode             string       `json:"mode"`
	MaxConcurrency   int          `json:"max_concurrency"`
	TimeoutSeconds   int          `json:"timeout_seconds"`
	Members          []teamMember `json:"members"`
	SynthesizerAgent string       `json:"synthesizer_agent_id"`
	A2A              struct {
		Enabled         *bool  `json:"enabled"`
		EnvelopeVersion string `json:"envelope_version"`
		MessageFormat   string `json:"message_format"`
		IncludeTrace    *bool  `json:"include_trace"`
		MaxPayloadChars int    `json:"max_payload_chars"`
	} `json:"a2a"`
	CriticLoop struct {
		MaxIterations  int     `json:"max_iterations"`
		ScoreThreshold float64 `json:"score_threshold"`
	} `json:"critic_loop"`
}

type teamMember struct {
	AgentID   string `json:"agent_id"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Enabled   *bool  `json:"enabled"`
	SortOrder int    `json:"sort_order"`
}

type teamStepResult struct {
	Member       teamMember
	Agent        domain.Agent
	Generated    adkr.GenerateResult
	CostMicroUSD int64
	Err          error
}

func (s *ChatService) sendTeam(ctx context.Context, in SendMessageInput, session domain.Session, callbacks *SendStreamCallbacks) (SendMessageResult, error) {
	teamID := firstNonEmptyString(session.TeamID, in.TeamID)
	if teamID == "" {
		return SendMessageResult{}, validationError("team_id is required")
	}
	team, err := s.repo.GetTeamByID(teamID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SendMessageResult{}, fmt.Errorf("%w: team %q was not found", errs.ErrNotFound, teamID)
		}
		return SendMessageResult{}, err
	}
	def := parseTeamDefinition(team.DefinitionJSON)
	members := enabledTeamMembers(def)
	if len(members) == 0 {
		return SendMessageResult{}, validationError("team has no enabled members")
	}
	optionsJSON := ""
	if in.Options.DialogMode != "" || in.Options.Provider != "" || in.Options.Model != "" || len(in.Options.Attachments) > 0 {
		raw, err := json.Marshal(in.Options)
		if err != nil {
			return SendMessageResult{}, err
		}
		optionsJSON = string(raw)
	}
	userMsg := domain.Message{
		ID:               newID(),
		SessionID:        in.SessionID,
		Role:             "user",
		Content:          in.Content,
		Status:           "ok",
		AttachmentsCount: len(in.Options.Attachments),
		OptionsJSON:      optionsJSON,
	}
	userMsg, err = s.repo.AddMessage(userMsg)
	if err != nil {
		return SendMessageResult{}, err
	}
	if callbacks != nil && callbacks.OnUserMessage != nil {
		if err = callbacks.OnUserMessage(userMsg); err != nil {
			return SendMessageResult{}, err
		}
	}

	history, err := s.repo.ListMessages(in.SessionID)
	if err != nil {
		return SendMessageResult{}, err
	}
	mode := strings.ToLower(strings.TrimSpace(def.Mode))
	if mode == "" {
		mode = "sequential"
	}
	if mode == "adaptive" {
		mode = selectAdaptiveTeamMode(def, members, in.Content)
	}
	runCtx := ctx
	cancelRun := func() {}
	if def.TimeoutSeconds > 0 {
		runCtx, cancelRun = context.WithTimeout(ctx, time.Duration(def.TimeoutSeconds)*time.Second)
	}
	defer cancelRun()
	now := nowUTC()
	run := domain.TeamRun{
		ID:           newID(),
		TeamID:       team.ID,
		SessionID:    session.ID,
		Mode:         mode,
		Status:       "running",
		InputPreview: previewText(in.Content, 240),
		TopologyJSON: firstNonEmptyString(team.DefinitionJSON, "{}"),
		StartedAt:    now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	run, err = s.repo.AddTeamRun(run)
	if err != nil {
		return SendMessageResult{}, err
	}
	s.publishTeamRunEvent(TeamRunEvent{Type: "run_started", TeamID: run.TeamID, RunID: run.ID, Run: &run})
	teamRC := s.teamRuntimeContext(session, team, def, members, mode, in.Options)
	steps, runErr := s.runTeamTopology(runCtx, run, def, members, in, session, history, mode, teamRC, callbacks)
	partialSuccess := runErr != nil && mode == "parallel" && hasSuccessfulTeamSteps(steps)
	if runErr != nil && !partialSuccess {
		run.Status = teamErrorStatus(runErr)
		run.ErrorMessage = runErr.Error()
		run.TokenIn = sumTeamPromptTokens(steps)
		run.TokenOut = sumTeamCompletionTokens(steps)
		run.CostMicroUSD = sumTeamCostMicroUSD(steps)
		run.DurationMS = sumTeamLatency(steps)
		run.FinishedAt = nowUTC()
		_, _ = s.repo.UpdateTeamRun(run)
		s.publishTeamRunEvent(TeamRunEvent{Type: "run_finished", TeamID: run.TeamID, RunID: run.ID, Run: &run})
		return SendMessageResult{}, runErr
	}
	content, steps, err := s.synthesizeTeamFinal(runCtx, run, def, team, mode, steps, in, session, history, teamRC, callbacks)
	if err != nil {
		run.Status = teamErrorStatus(err)
		run.ErrorMessage = err.Error()
		run.TokenIn = sumTeamPromptTokens(steps)
		run.TokenOut = sumTeamCompletionTokens(steps)
		run.CostMicroUSD = sumTeamCostMicroUSD(steps)
		run.DurationMS = sumTeamLatency(steps)
		run.FinishedAt = nowUTC()
		_, _ = s.repo.UpdateTeamRun(run)
		s.publishTeamRunEvent(TeamRunEvent{Type: "run_finished", TeamID: run.TeamID, RunID: run.ID, Run: &run})
		return SendMessageResult{}, err
	}
	if callbacks != nil && callbacks.OnDelta != nil {
		if err = callbacks.OnDelta(content); err != nil {
			return SendMessageResult{}, err
		}
	}
	agentMsg := domain.Message{
		ID:        newID(),
		SessionID: in.SessionID,
		Role:      "assistant",
		Content:   content,
		ModelName: "team/" + mode,
		TokenIn:   sumTeamPromptTokens(steps),
		TokenOut:  sumTeamCompletionTokens(steps),
		LatencyMS: sumTeamLatency(steps),
		Status:    "ok",
	}
	agentMsg, err = s.repo.AddMessage(agentMsg)
	if err != nil {
		return SendMessageResult{}, err
	}
	run.MessageID = agentMsg.ID
	if partialSuccess || hasFailedTeamSteps(steps) {
		run.Status = "partial_success"
		run.ErrorMessage = firstTeamStepError(steps)
	} else {
		run.Status = "success"
	}
	run.OutputPreview = previewText(content, 300)
	run.TokenIn = agentMsg.TokenIn
	run.TokenOut = agentMsg.TokenOut
	run.CostMicroUSD = sumTeamCostMicroUSD(steps)
	run.DurationMS = agentMsg.LatencyMS
	run.FinishedAt = nowUTC()
	_, _ = s.repo.UpdateTeamRun(run)
	s.publishTeamRunEvent(TeamRunEvent{Type: "run_finished", TeamID: run.TeamID, RunID: run.ID, Run: &run})
	if callbacks != nil && callbacks.OnAgentMessage != nil {
		if err = callbacks.OnAgentMessage(agentMsg); err != nil {
			return SendMessageResult{}, err
		}
	}
	return SendMessageResult{UserMessage: userMsg, AgentMessage: agentMsg}, nil
}

func (s *ChatService) runTeamTopology(ctx context.Context, run domain.TeamRun, def teamDefinition, members []teamMember, in SendMessageInput, session domain.Session, history []domain.Message, mode string, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) ([]teamStepResult, error) {
	switch mode {
	case "parallel":
		return s.runTeamParallel(ctx, run, members, in, session, history, def.MaxConcurrency, teamRC, callbacks)
	case "coordinator":
		return s.runTeamCoordinator(ctx, run, members, in, session, history, teamRC, callbacks)
	case "critic_loop":
		return s.runTeamCriticLoop(ctx, run, def, members, in, session, history, teamRC, callbacks)
	default:
		return s.runTeamSequential(ctx, run, def, members, in, session, history, in.Content, teamRC, callbacks)
	}
}

func selectAdaptiveTeamMode(def teamDefinition, members []teamMember, task string) string {
	normalized := strings.ToLower(task)
	hasCoordinator := hasTeamRole(members, "coordinator")
	hasGenerator := hasTeamRole(members, "generator")
	hasCritic := hasTeamRole(members, "critic")
	if hasGenerator && hasCritic && containsAny(normalized, []string{"评审", "审核", "修改", "优化", "review", "revise", "critique"}) {
		return "critic_loop"
	}
	if hasCoordinator && containsAny(normalized, []string{"规划", "计划", "拆解", "分解", "复杂", "多步骤", "plan", "coordinate", "breakdown"}) {
		return "coordinator"
	}
	if len(members) >= 3 {
		return "parallel"
	}
	return "sequential"
}

func hasTeamRole(members []teamMember, role string) bool {
	for _, member := range members {
		if strings.EqualFold(member.Role, role) {
			return true
		}
	}
	return false
}

func containsAny(value string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(value, term) {
			return true
		}
	}
	return false
}

func parseTeamDefinition(raw string) teamDefinition {
	def := teamDefinition{Mode: "sequential", MaxConcurrency: 2}
	if strings.TrimSpace(raw) == "" {
		return def
	}
	_ = json.Unmarshal([]byte(raw), &def)
	if def.Mode == "" {
		def.Mode = "sequential"
	}
	return def
}

func enabledTeamMembers(def teamDefinition) []teamMember {
	items := make([]teamMember, 0, len(def.Members))
	for _, member := range def.Members {
		if member.Enabled != nil && !*member.Enabled {
			continue
		}
		items = append(items, member)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].SortOrder < items[j].SortOrder
	})
	return items
}

func (s *ChatService) runTeamSequential(ctx context.Context, run domain.TeamRun, def teamDefinition, members []teamMember, in SendMessageInput, session domain.Session, history []domain.Message, input string, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) ([]teamStepResult, error) {
	steps := make([]teamStepResult, 0, len(members))
	current := buildA2AEnvelope(def, teamMember{Name: "User", Role: "user"}, members[0], "task", in.Content, input, run.ID)
	for index, member := range members {
		if err := ctx.Err(); err != nil {
			step := teamStepResult{Member: member, Err: err}
			steps = append(steps, step)
			_, _ = s.recordTeamRunStep(run, step, index)
			return steps, err
		}
		step, err := s.generateTeamStep(ctx, run, index, member, in, session, history, current, teamRC, callbacks)
		steps = append(steps, step)
		_, _ = s.recordTeamRunStep(run, step, index)
		if err != nil {
			return steps, err
		}
		if index+1 < len(members) {
			current = buildA2AEnvelope(def, member, members[index+1], "handoff", in.Content, step.Generated.Content, run.ID)
		}
	}
	return steps, nil
}

func (s *ChatService) runTeamParallel(ctx context.Context, run domain.TeamRun, members []teamMember, in SendMessageInput, session domain.Session, history []domain.Message, maxConcurrency int, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) ([]teamStepResult, error) {
	if maxConcurrency <= 0 {
		maxConcurrency = len(members)
	}
	sem := make(chan struct{}, maxConcurrency)
	steps := make([]teamStepResult, len(members))
	var wg sync.WaitGroup
	for i, member := range members {
		wg.Add(1)
		go func(index int, item teamMember) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				steps[index] = teamStepResult{Member: item, Err: ctx.Err()}
				_, _ = s.recordTeamRunStep(run, steps[index], index)
				return
			}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				steps[index] = teamStepResult{Member: item, Err: err}
				_, _ = s.recordTeamRunStep(run, steps[index], index)
				return
			}
			input := buildA2AEnvelope(parseTeamDefinition(run.TopologyJSON), teamMember{Name: "User", Role: "user"}, item, "parallel_task", in.Content, in.Content, run.ID)
			step, err := s.generateTeamStep(ctx, run, index, item, in, session, history, input, teamRC, callbacks)
			if err != nil && step.Err == nil {
				step.Err = err
			}
			steps[index] = step
			_, _ = s.recordTeamRunStep(run, step, index)
		}(i, member)
	}
	wg.Wait()
	for _, step := range steps {
		if step.Err != nil {
			if hasSuccessfulTeamSteps(steps) {
				return steps, nil
			}
			return steps, step.Err
		}
	}
	return steps, nil
}

func (s *ChatService) runTeamCoordinator(ctx context.Context, run domain.TeamRun, members []teamMember, in SendMessageInput, session domain.Session, history []domain.Message, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) ([]teamStepResult, error) {
	coordinator, workers := splitTeamRole(members, "coordinator")
	if coordinator.AgentID == "" {
		coordinator = members[0]
		workers = members[1:]
	}
	planPrompt := buildA2AEnvelope(parseTeamDefinition(run.TopologyJSON), teamMember{Name: "User", Role: "user"}, coordinator, "plan_request", in.Content, "请把用户任务拆解为可执行计划，明确每个成员应完成的工作、依赖和最终汇总口径。", run.ID)
	planStep, err := s.generateTeamStep(ctx, run, 0, coordinator, in, session, history, planPrompt, teamRC, callbacks)
	steps := []teamStepResult{planStep}
	_, _ = s.recordTeamRunStep(run, planStep, 0)
	if err != nil {
		return steps, err
	}
	if len(workers) == 0 {
		return steps, nil
	}
	current := fmt.Sprintf("用户任务：%s\n\nCoordinator 计划：\n%s\n\n请按你的角色完成计划中分配给你的部分。", in.Content, planStep.Generated.Content)
	for index, member := range workers {
		workerInput := buildA2AEnvelope(parseTeamDefinition(run.TopologyJSON), coordinator, member, "delegation", in.Content, current, run.ID)
		step, err := s.generateTeamStep(ctx, run, index+1, member, in, session, history, workerInput, teamRC, callbacks)
		steps = append(steps, step)
		_, _ = s.recordTeamRunStep(run, step, index+1)
		if err != nil {
			return steps, err
		}
		current += "\n\n上一位成员输出：\n" + step.Generated.Content
	}
	return steps, nil
}

func (s *ChatService) runTeamCriticLoop(ctx context.Context, run domain.TeamRun, def teamDefinition, members []teamMember, in SendMessageInput, session domain.Session, history []domain.Message, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) ([]teamStepResult, error) {
	generator, remaining := splitTeamRole(members, "generator")
	if generator.AgentID == "" {
		generator = members[0]
		remaining = members[1:]
	}
	critic, _ := splitTeamRole(remaining, "critic")
	if critic.AgentID == "" && len(remaining) > 0 {
		critic = remaining[0]
	}
	maxIterations := def.CriticLoop.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 1
	}
	if maxIterations > 3 {
		maxIterations = 3
	}

	steps := []teamStepResult{}
	defForEnvelope := parseTeamDefinition(run.TopologyJSON)
	draftPrompt := buildA2AEnvelope(defForEnvelope, teamMember{Name: "User", Role: "user"}, generator, "draft_request", in.Content, "请先产出可评审的初稿。", run.ID)
	draft, err := s.generateTeamStep(ctx, run, 0, generator, in, session, history, draftPrompt, teamRC, callbacks)
	steps = append(steps, draft)
	_, _ = s.recordTeamRunStep(run, draft, 0)
	if err != nil || critic.AgentID == "" {
		return steps, err
	}
	currentDraft := draft.Generated.Content
	for iteration := 1; iteration <= maxIterations; iteration++ {
		criticPayload := fmt.Sprintf("请评审第 %d 轮初稿，指出是否通过、关键问题和修改建议。\n\n初稿：\n%s", iteration, currentDraft)
		criticPrompt := buildA2AEnvelope(defForEnvelope, generator, critic, "review_request", in.Content, criticPayload, run.ID)
		review, err := s.generateTeamStep(ctx, run, len(steps), critic, in, session, history, criticPrompt, teamRC, callbacks)
		steps = append(steps, review)
		_, _ = s.recordTeamRunStep(run, review, len(steps)-1)
		if err != nil {
			return steps, err
		}
		if iteration == maxIterations || !criticNeedsRevision(review.Generated.Content) {
			break
		}
		revisionPayload := fmt.Sprintf("请根据 critic 意见修订初稿，输出完整最终稿。\n\n当前初稿：\n%s\n\nCritic 意见：\n%s", currentDraft, review.Generated.Content)
		revisionPrompt := buildA2AEnvelope(defForEnvelope, critic, generator, "revision_request", in.Content, revisionPayload, run.ID)
		revision, err := s.generateTeamStep(ctx, run, len(steps), generator, in, session, history, revisionPrompt, teamRC, callbacks)
		steps = append(steps, revision)
		_, _ = s.recordTeamRunStep(run, revision, len(steps)-1)
		if err != nil {
			return steps, err
		}
		currentDraft = revision.Generated.Content
	}
	return steps, nil
}

func splitTeamRole(members []teamMember, role string) (teamMember, []teamMember) {
	rest := make([]teamMember, 0, len(members))
	var matched teamMember
	for _, member := range members {
		if matched.AgentID == "" && strings.EqualFold(member.Role, role) {
			matched = member
			continue
		}
		rest = append(rest, member)
	}
	return matched, rest
}

func criticNeedsRevision(content string) bool {
	normalized := strings.ToLower(content)
	revisionTerms := []string{"不通过", "修改", "问题", "不足", "revision", "revise", "fail"}
	for _, term := range revisionTerms {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	passTerms := []string{"通过", "无需修改", "pass", "approved"}
	for _, term := range passTerms {
		if strings.Contains(normalized, term) {
			return false
		}
	}
	return false
}

func (s *ChatService) generateTeamStep(ctx context.Context, run domain.TeamRun, index int, member teamMember, in SendMessageInput, session domain.Session, history []domain.Message, input string, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) (teamStepResult, error) {
	agent, err := s.repo.GetAgentByID(member.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("%w: team member agent %q was not found", errs.ErrNotFound, member.AgentID)
		}
		return teamStepResult{Member: member, Err: err}, err
	}
	provider, model := resolveProviderModel(in.Options, session, agent)
	providerModel, err := s.repo.GetProviderModel(provider, model)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = fmt.Errorf("%w: provider model %q/%q is not enabled or does not exist", errs.ErrNotFound, provider, model)
		}
		return teamStepResult{Member: member, Agent: agent, Err: err}, err
	}
	roleName := firstNonEmptyString(member.Name, member.Role, agent.DisplayName)
	prompt := fmt.Sprintf("你是 Team 成员「%s」。请基于你的专业角色处理以下任务，并输出清晰结果。\n\n%s", roleName, input)
	messageID := newID()
	if callbacks != nil && callbacks.OnTeamMemberStart != nil {
		if err = callbacks.OnTeamMemberStart(teamMemberMessageShell(session.ID, messageID, member, agent, "streaming")); err != nil {
			return teamStepResult{Member: member, Agent: agent, Err: err}, err
		}
	}

	// 每个子智能体独立 L0 组装，提示窗口、摘要与 L1/L3/L4 开关互不影响（规范 F10）。
	// L0 不可用时仍保留历史+纯文本组装的旧路径，避免团队运行回退。
	var (
		messages []adkr.ChatMessage
		l0Result mem.L0AssemblyResult
	)
	if s.memoryL0 != nil {
		req := mem.L0AssemblyRequest{
			SessionID:     session.ID,
			AgentID:       agent.ID,
			TeamID:        session.TeamID,
			Provider:      providerModel.Provider,
			Model:         providerModel.Model,
			ContextWindow: providerContextWindowTokens(providerModel, agent),
			UserMessage:   prompt,
		}
		if res, l0Err := s.memoryL0.Assemble(ctx, req); l0Err == nil {
			l0Result = res
			messages = make([]adkr.ChatMessage, 0, len(res.PromptMessages))
			for _, m := range res.PromptMessages {
				messages = append(messages, adkr.ChatMessage{Role: m.Role, Content: m.Content})
			}
		}
	}
	if messages == nil {
		messages = make([]adkr.ChatMessage, 0, len(history)+1)
		for _, item := range history {
			if item.Role != "user" && item.Role != "assistant" {
				continue
			}
			messages = append(messages, adkr.ChatMessage{Role: item.Role, Content: item.Content})
		}
		messages = append(messages, adkr.ChatMessage{Role: "user", Content: prompt})
	}

	memberRoleLabel := firstNonEmptyString(member.Role, "member")
	memberRC := teamRC.CloneWithRole(memberRoleLabel)
	req := adkr.GenerateRequest{
		Agent:          agent,
		ProviderModel:  providerModel,
		Messages:       messages,
		Input:          prompt,
		ToolSettings:   s.runtimeToolSettings(agent.ID),
		RuntimeContext: memberRC,
	}
	if callbacks != nil {
		req.OnToolEvent = func(event adkr.ToolEvent) error {
			s.recordToolEvent(session.ID, messageID, event)
			if callbacks.OnToolEvent != nil {
				return callbacks.OnToolEvent(event)
			}
			return nil
		}
	}
	var generated adkr.GenerateResult
	if callbacks != nil && callbacks.OnTeamMemberDelta != nil {
		generated, err = s.runtime.StreamGenerate(ctx, req, func(delta string) error {
			return callbacks.OnTeamMemberDelta(messageID, delta)
		})
	} else {
		generated, err = s.runtime.Generate(ctx, req)
	}
	if err != nil {
		_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, adkr.GenerateResult{}, domain.Message{}, callbacks != nil, teamErrorStatus(err), err)
		_, _ = s.recordTeamMemberMessage(session.ID, messageID, member, agent, adkr.GenerateResult{}, err, callbacks)
		return teamStepResult{Member: member, Agent: agent, Err: err}, err
	}
	costMicroUSD := s.estimateGeneratedCost(providerModel, generated)
	memberMessage, err := s.recordTeamMemberMessage(session.ID, messageID, member, agent, generated, nil, callbacks)
	if err != nil {
		return teamStepResult{Member: member, Agent: agent, Err: err}, err
	}
	_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, generated, memberMessage, callbacks != nil, "success", nil)
	_ = s.recordL0Actual(ctx, session.ID, agent, providerModel, l0Result, generated)
	return teamStepResult{Member: member, Agent: agent, Generated: generated, CostMicroUSD: costMicroUSD}, nil
}

func teamMemberMessageShell(sessionID string, messageID string, member teamMember, agent domain.Agent, status string) domain.Message {
	return domain.Message{
		ID:          messageID,
		SessionID:   sessionID,
		Role:        "assistant",
		Content:     "",
		ModelName:   teamMemberModelName(member, agent),
		Status:      status,
		OptionsJSON: teamMemberMessageOptions(member, agent),
		CreatedAt:   nowUTC(),
	}
}

func (s *ChatService) recordTeamMemberMessage(sessionID string, messageID string, member teamMember, agent domain.Agent, generated adkr.GenerateResult, stepErr error, callbacks *SendStreamCallbacks) (domain.Message, error) {
	content := generated.Content
	status := "ok"
	if stepErr != nil {
		status = teamErrorStatus(stepErr)
		content = "执行失败：" + stepErr.Error()
	}
	message := domain.Message{
		ID:          messageID,
		SessionID:   sessionID,
		Role:        "assistant",
		Content:     content,
		ModelName:   teamMemberModelName(member, agent),
		TokenIn:     generated.PromptTokens,
		TokenOut:    generated.CompletionTokens,
		LatencyMS:   generated.LatencyMS,
		Status:      status,
		OptionsJSON: teamMemberMessageOptions(member, agent),
	}
	created, err := s.repo.AddMessage(message)
	if err != nil {
		return domain.Message{}, err
	}
	if callbacks != nil && callbacks.OnTeamMemberMessage != nil {
		if err = callbacks.OnTeamMemberMessage(created); err != nil {
			return domain.Message{}, err
		}
	}
	return created, nil
}

func teamMemberModelName(member teamMember, agent domain.Agent) string {
	label := firstNonEmptyString(member.Name, agent.DisplayName, member.Role, agent.AgentKey, member.AgentID)
	role := firstNonEmptyString(member.Role, "member")
	return "team/" + role + "/" + label
}

func teamMemberMessageOptions(member teamMember, agent domain.Agent) string {
	raw, err := json.Marshal(map[string]any{
		"team_member": map[string]string{
			"agent_id":  agent.ID,
			"agent_key": agent.AgentKey,
			"name":      firstNonEmptyString(member.Name, agent.DisplayName, agent.AgentKey, member.AgentID),
			"role":      firstNonEmptyString(member.Role, "member"),
			"icon":      agent.Icon,
		},
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func (s *ChatService) recordTeamRunStep(run domain.TeamRun, step teamStepResult, index int) (domain.TeamRunStep, error) {
	status := "success"
	errorMessage := ""
	if step.Err != nil {
		status = teamErrorStatus(step.Err)
		errorMessage = step.Err.Error()
	}
	sortOrder := step.Member.SortOrder
	if sortOrder == 0 {
		sortOrder = index + 1
	}
	now := nowUTC()
	item := domain.TeamRunStep{
		ID:            newID(),
		RunID:         run.ID,
		TeamID:        run.TeamID,
		AgentID:       step.Member.AgentID,
		AgentKey:      step.Agent.AgentKey,
		AgentName:     firstNonEmptyString(step.Member.Name, step.Agent.DisplayName),
		Role:          step.Member.Role,
		SortOrder:     sortOrder,
		Status:        status,
		InputPreview:  previewText(run.InputPreview, 240),
		OutputPreview: previewText(step.Generated.Content, 300),
		TokenIn:       step.Generated.PromptTokens,
		TokenOut:      step.Generated.CompletionTokens,
		CostMicroUSD:  step.CostMicroUSD,
		DurationMS:    step.Generated.LatencyMS,
		ErrorMessage:  errorMessage,
		StartedAt:     now,
		FinishedAt:    now,
		CreatedAt:     now,
	}
	created, err := s.repo.AddTeamRunStep(item)
	if err == nil {
		s.publishTeamRunEvent(TeamRunEvent{Type: "step_finished", TeamID: run.TeamID, RunID: run.ID, Step: &created})
	}
	return created, err
}

func (s *ChatService) synthesizeTeamFinal(ctx context.Context, run domain.TeamRun, def teamDefinition, team domain.Team, mode string, steps []teamStepResult, in SendMessageInput, session domain.Session, history []domain.Message, teamRC *runctx.RuntimeContext, callbacks *SendStreamCallbacks) (string, []teamStepResult, error) {
	if strings.TrimSpace(def.SynthesizerAgent) == "" {
		return synthesizeTeamOutput(team, mode, steps), steps, nil
	}
	if !hasSuccessfulTeamSteps(steps) {
		return synthesizeTeamOutput(team, mode, steps), steps, nil
	}
	member := teamMember{
		AgentID:   def.SynthesizerAgent,
		Role:      "synthesizer",
		Name:      "Synthesizer",
		SortOrder: maxTeamStepSortOrder(steps) + 10,
	}
	prompt := buildSynthesizerPrompt(team, mode, in.Content, steps)
	step, err := s.generateTeamStep(ctx, run, len(steps), member, in, session, history, prompt, teamRC, callbacks)
	steps = append(steps, step)
	_, _ = s.recordTeamRunStep(run, step, len(steps)-1)
	if err != nil {
		return synthesizeTeamOutput(team, mode, steps), steps, err
	}
	return step.Generated.Content, steps, nil
}

func buildSynthesizerPrompt(team domain.Team, mode string, userInput string, steps []teamStepResult) string {
	var b strings.Builder
	b.WriteString("你是 Team 的 synthesizer。请根据各成员结果生成最终回复，避免机械拼接，保留失败说明和可执行结论。\n\n")
	b.WriteString("Team：")
	b.WriteString(team.DisplayName)
	b.WriteString("\n编排模式：")
	b.WriteString(mode)
	b.WriteString("\n用户任务：")
	b.WriteString(userInput)
	b.WriteString("\n\n成员输出：\n")
	for i, step := range steps {
		b.WriteString("\n## ")
		b.WriteString(fmt.Sprintf("%d. %s", i+1, firstNonEmptyString(step.Member.Name, step.Member.Role, step.Agent.DisplayName, step.Member.AgentID)))
		b.WriteString("\n状态：")
		if step.Err != nil {
			b.WriteString("failed\n错误：")
			b.WriteString(step.Err.Error())
			b.WriteString("\n")
			continue
		}
		b.WriteString("success\n输出：\n")
		b.WriteString(step.Generated.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func buildA2AEnvelope(def teamDefinition, sender teamMember, receiver teamMember, intent string, userTask string, payload string, runID string) string {
	if def.A2A.Enabled != nil && !*def.A2A.Enabled {
		return payload
	}
	maxPayloadChars := def.A2A.MaxPayloadChars
	if maxPayloadChars <= 0 {
		maxPayloadChars = 6000
	}
	payload = previewText(payload, maxPayloadChars)
	version := firstNonEmptyString(def.A2A.EnvelopeVersion, "a2a.v1")
	format := firstNonEmptyString(def.A2A.MessageFormat, "markdown_json")
	if format == "plain" {
		return fmt.Sprintf("A2A %s\nFrom: %s\nTo: %s\nIntent: %s\nRun: %s\n\nUser Task:\n%s\n\nPayload:\n%s", version, a2aActorLabel(sender), a2aActorLabel(receiver), intent, runID, userTask, payload)
	}
	body := map[string]any{
		"version": version,
		"run_id":  runID,
		"intent":  intent,
		"sender": map[string]string{
			"agent_id": sender.AgentID,
			"role":     sender.Role,
			"name":     sender.Name,
		},
		"receiver": map[string]string{
			"agent_id": receiver.AgentID,
			"role":     receiver.Role,
			"name":     receiver.Name,
		},
		"user_task": userTask,
		"payload":   payload,
	}
	includeTrace := true
	if def.A2A.IncludeTrace != nil {
		includeTrace = *def.A2A.IncludeTrace
	}
	if includeTrace {
		body["trace"] = map[string]string{"protocol": "team.a2a", "format": format}
	}
	raw, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return payload
	}
	return "请读取以下 A2A 消息信封，并仅以接收方角色完成任务。\n\n```json\n" + string(raw) + "\n```"
}

func a2aActorLabel(member teamMember) string {
	return firstNonEmptyString(member.Name, member.Role, member.AgentID, "unknown")
}

func hasSuccessfulTeamSteps(steps []teamStepResult) bool {
	for _, step := range steps {
		if step.Err == nil && strings.TrimSpace(step.Generated.Content) != "" {
			return true
		}
	}
	return false
}

func hasFailedTeamSteps(steps []teamStepResult) bool {
	for _, step := range steps {
		if step.Err != nil {
			return true
		}
	}
	return false
}

func firstTeamStepError(steps []teamStepResult) string {
	for _, step := range steps {
		if step.Err != nil {
			return step.Err.Error()
		}
	}
	return ""
}

func teamErrorStatus(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "failed"
}

func maxTeamStepSortOrder(steps []teamStepResult) int {
	maxOrder := 0
	for index, step := range steps {
		order := step.Member.SortOrder
		if order == 0 {
			order = index + 1
		}
		if order > maxOrder {
			maxOrder = order
		}
	}
	return maxOrder
}

func synthesizeTeamOutput(team domain.Team, mode string, steps []teamStepResult) string {
	successCount := 0
	failedCount := 0
	for _, step := range steps {
		if step.Err != nil {
			failedCount++
		} else {
			successCount++
		}
	}
	var b strings.Builder
	b.WriteString("## Team 协作完成\n\n")
	b.WriteString("- Team：")
	b.WriteString(team.DisplayName)
	b.WriteString("\n- 编排模式：")
	b.WriteString(mode)
	b.WriteString("\n- 动态拓扑：")
	b.WriteString(teamTopologyLabel(mode))
	b.WriteString(fmt.Sprintf("\n- 成员结果：%d 个成功，%d 个失败\n\n", successCount, failedCount))
	b.WriteString("各成员的详细输出已作为独立消息卡片展示。")
	return strings.TrimSpace(b.String())
}

func teamTopologyLabel(mode string) string {
	switch mode {
	case "parallel":
		return "并行分派 -> 汇总"
	case "coordinator":
		return "主控拆分 -> 成员执行 -> 汇总"
	case "critic_loop":
		return "生成 -> 评审 -> 迭代"
	default:
		return "顺序流水线"
	}
}

func sumTeamPromptTokens(steps []teamStepResult) int {
	total := 0
	for _, step := range steps {
		total += step.Generated.PromptTokens
	}
	return total
}

func sumTeamCompletionTokens(steps []teamStepResult) int {
	total := 0
	for _, step := range steps {
		total += step.Generated.CompletionTokens
	}
	return total
}

func sumTeamCostMicroUSD(steps []teamStepResult) int64 {
	var total int64
	for _, step := range steps {
		total += step.CostMicroUSD
	}
	return total
}

func sumTeamLatency(steps []teamStepResult) int {
	total := 0
	for _, step := range steps {
		total += step.Generated.LatencyMS
	}
	return total
}

func (s *ChatService) estimateGeneratedCost(providerModel domain.PlatformResource, generated adkr.GenerateResult) int64 {
	pricing, err := s.repo.GetActiveModelPricingRule(providerModel.Provider, providerModel.Model, nowUTC())
	if err != nil {
		return 0
	}
	inputCost := costMicroUSD(generated.PromptTokens, pricing.InputPriceMicroUSDPer1K)
	outputCost := costMicroUSD(generated.CompletionTokens, pricing.OutputPriceMicroUSDPer1K)
	return inputCost + outputCost
}

// teamRuntimeContext 构建团队运行中所有成员共享的结构化运行时上下文。
// 记录团队成员、编排模式与会话元数据。各成员在调用 LLM 前
// 用自身角色克隆此上下文。
func (s *ChatService) teamRuntimeContext(session domain.Session, team domain.Team, def teamDefinition, members []teamMember, mode string, options SendMessageOptions) *runctx.RuntimeContext {
	dialogMode := strings.TrimSpace(options.DialogMode)
	if dialogMode == "" {
		dialogMode = strings.TrimSpace(session.DialogMode)
	}
	teamMembers := make([]runctx.TeamMemberContext, 0, len(members))
	for _, member := range members {
		name := firstNonEmptyString(member.Name, member.Role, member.AgentID)
		teamMembers = append(teamMembers, runctx.TeamMemberContext{
			AgentID: member.AgentID,
			Role:    firstNonEmptyString(member.Role, "member"),
			Name:    name,
		})
	}
	return &runctx.RuntimeContext{
		Session: runctx.SessionContext{
			SessionID:  session.ID,
			DialogMode: dialogMode,
			StartedAt:  session.CreatedAt,
		},
		Team: &runctx.TeamContext{
			TeamID:      team.ID,
			DisplayName: firstNonEmptyString(team.DisplayName, team.TeamKey),
			Mode:        firstNonEmptyString(mode, def.Mode),
			Members:     teamMembers,
		},
	}
}
