package application

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

// MemoryL0Service 为 `12 memory-L0-sensory.md` 中的组装层。
// 不自有数据：历史在 `messages`，摘要在 `session_summaries`，智能体提示来自 `agent_prompt_files`。
// 本服务只负责排序、token 预算、截断与快照写入，使 ChatService/TeamRuntime 每次调用向 LLM 提供干净的 `messages` 切片。
type MemoryL0Service struct {
	repo      repository.Store
	memoryL1  L1PromptSource
	memoryL2  L2RecallSource
	memoryL3  L3RecallSource
	memoryL4  L4RecallSource
	evolution EvolutionPromptSource
}

// L1PromptSource 为 MemoryL0Service 渲染可选 L1 工作记忆片段的窄接口。完整 MemoryL1Service（memory_l1_service.go）实现之；间接层避免循环导入，并允许从 ChatService 注入。
type L1PromptSource interface {
	RenderActiveTaskForPrompt(ctx context.Context, sessionID, agentID string) (mem.L1PromptBlock, bool, error)
}

// L2RecallSource 为 MemoryL0Service 渲染可选 L2 情节回忆片段的窄接口，见
// `aranea/docs/14 memory-L2-episodic.md` §5.3/§5.4。由 *MemoryL2Service 实现。当召回关闭（agent_runtime_settings 默认）时，该接缝使 L0 主路径无分支。
type L2RecallSource interface {
	RecallSegmentForL0(ctx context.Context, sessionID, agentID, query string) (mem.L0Segment, bool)
}

// L3RecallSource 为 MemoryL0Service 渲染可选 L3 语义记忆片段的窄接口，见
// `aranea/docs/15 memory-L3-semantic.md` §5.3/§7。由 *MemoryL3Service 实现。召回关闭时 L0 主路径无分支。
type L3RecallSource interface {
	RecallSegmentForL0WithContext(ctx context.Context, scope mem.L0MemoryScopeContext) (mem.L0Segment, bool)
}

// L4RecallSource 为 MemoryL0Service 渲染可选 L4 知识图邻域片段的窄接口，见
// `aranea/docs/16 memory-L4-persistent.md` §5.7/§10。由 *MemoryL4Service 实现。功能关闭时 L0 主路径无分支。
type L4RecallSource interface {
	NeighborhoodSegmentForL0WithContext(ctx context.Context, scope mem.L0MemoryScopeContext) (mem.L0Segment, bool)
}

// EvolutionPromptSource 为 MemoryL0Service 将自进化片段（persona/价值观/语气/策略提示）注入系统提示的窄接口。由 *AgentEvolutionService 实现。
type EvolutionPromptSource interface {
	BuildSelfPromptAppend(ctx context.Context, agentID string) (string, error)
}

func NewMemoryL0Service(repo repository.Store) *MemoryL0Service {
	return &MemoryL0Service{repo: repo}
}

// SetL1Source 将 MemoryL1Service 接入 L0 组装流水线。可选：nil 则省略 L1 片段。
func (s *MemoryL0Service) SetL1Source(src L1PromptSource) {
	s.memoryL1 = src
}

// SetL2Source 将 MemoryL2Service 接入 L0 组装流水线。可选：nil 则省略 L2 召回片段。另受 agent_runtime_settings 的 `l2_recall_enabled` 门控。
func (s *MemoryL0Service) SetL2Source(src L2RecallSource) {
	s.memoryL2 = src
}

// SetL3Source 将 MemoryL3Service 接入 L0 组装流水线。可选：nil 则省略 L3 召回片段。另受 `l3_enabled` 与 `l0_inject_l3` 门控。
func (s *MemoryL0Service) SetL3Source(src L3RecallSource) {
	s.memoryL3 = src
}

// SetL4Source 将 MemoryL4Service 接入 L0 组装流水线。可选：nil 则省略 L4 图谱片段。另受 `l4_enabled` / `l4_graph_inject_neighbors` 门控。
func (s *MemoryL0Service) SetL4Source(src L4RecallSource) {
	s.memoryL4 = src
}

// SetEvolutionSource 将 AgentEvolutionService 接入 L0 组装流水线。可选：nil 则省略自进化系统片段。另受 `l4_enabled` / `l4_identity_inject` / `l4_strategy_inject` 门控。
func (s *MemoryL0Service) SetEvolutionSource(src EvolutionPromptSource) {
	s.evolution = src
}

// l0DefaultSafetyMargin 从模型上下文窗口中预留数百 token，用于输出包装、函数调用模式与 SSE 开销。预算公式：max(0, context_window - reserved_for_output - safety)。
const l0DefaultSafetyMargin = 256

// l0PreviewLimit 限制写入快照及 /l0/preview 返回的 `Preview` 字符数。刻意较短——完整内容已在 `messages` / `session_summaries`。
const l0PreviewLimit = 200

// l0HardMessageCap 限制每窗口扫描消息数上限，即使 token 未达预算（防止会话失控）。
const l0HardMessageCap = 200

// Assemble 按请求构建可送模型的提示。snapshot_mode=off 且无告警时返回的 SnapshotID 为空。
func (s *MemoryL0Service) Assemble(ctx context.Context, req mem.L0AssemblyRequest) (mem.L0AssemblyResult, error) {
	return s.assemble(ctx, req, false)
}

// Preview 与 Assemble 相同但不持久化快照、不标记 session.context_status，并将各片段脱敏为预览长度。供 `/l0/preview` 调试 API 与应用内提示调试器。
func (s *MemoryL0Service) Preview(ctx context.Context, req mem.L0AssemblyRequest) (mem.L0AssemblyResult, error) {
	return s.assemble(ctx, req, true)
}

// RecordActual 在已知模型用量后调用，使快照记录真实提示 token 数，会话可更新比例/状态。未写快照时传空 snapshotID 安全。
func (s *MemoryL0Service) RecordActual(ctx context.Context, sessionID string, snapshotID string, actualPromptTokens int, contextWindow int) error {
	if sessionID == "" {
		return errors.New("session id is required")
	}
	ratio := 0.0
	if contextWindow > 0 && actualPromptTokens > 0 {
		ratio = float64(actualPromptTokens) / float64(contextWindow)
		if math.IsInf(ratio, 0) || math.IsNaN(ratio) {
			ratio = 0
		}
		if ratio > 1 {
			ratio = 1
		}
	}
	if err := s.repo.UpdateSessionL0Context(sessionID, actualPromptTokens, contextWindow, ratio); err != nil {
		return err
	}
	if snapshotID == "" {
		return nil
	}
	return s.repo.UpdateL0AssemblySnapshotActualTokens(snapshotID, actualPromptTokens, ratio)
}

func (s *MemoryL0Service) ListSnapshots(ctx context.Context, sessionID string, limit int) ([]mem.L0AssemblySnapshot, error) {
	return s.repo.ListL0AssemblySnapshotsBySession(sessionID, limit)
}

func (s *MemoryL0Service) GetSnapshot(ctx context.Context, id string) (mem.L0AssemblySnapshot, error) {
	return s.repo.GetL0AssemblySnapshotByID(id)
}

// assemble 为 Assemble/Preview 的共享核心。`previewMode` 为真时从返回片段去掉完整 `Content` 并跳过持久化。
func (s *MemoryL0Service) assemble(ctx context.Context, req mem.L0AssemblyRequest, previewMode bool) (mem.L0AssemblyResult, error) {
	if req.SessionID == "" {
		return mem.L0AssemblyResult{}, errors.New("session_id is required")
	}

	settings := s.resolveL0Settings(req.AgentID)

	contextWindow := req.ContextWindow
	if contextWindow <= 0 {
		contextWindow = 32_000
	}
	reservedOutput := req.ReservedForOutput
	if reservedOutput < 0 {
		reservedOutput = 0
	}
	budget := contextWindow - reservedOutput - l0DefaultSafetyMargin
	if budget <= 0 {
		budget = contextWindow / 2
	}

	segments := make([]mem.L0Segment, 0, 16)

	systemSegments, err := s.buildSystemSegments(ctx, req.AgentID, req.ExtraSystemBlocks)
	if err != nil {
		return mem.L0AssemblyResult{}, err
	}
	segments = append(segments, systemSegments...)

	if settings.InjectL1 {
		if seg, ok := s.buildL1Segment(ctx, req.SessionID, req.AgentID); ok {
			segments = append(segments, seg)
		}
	}
	if seg, ok := s.buildL2Segment(ctx, req.SessionID, req.AgentID, req.UserMessage); ok {
		segments = append(segments, seg)
	}
	scope := s.memoryScopeContext(req)
	if settings.InjectL3 {
		if seg, ok := s.buildL3Segment(ctx, scope); ok {
			segments = append(segments, seg)
		}
	}
	if settings.InjectL4 {
		if seg, ok := s.buildL4Segment(ctx, scope); ok {
			segments = append(segments, seg)
		}
	}

	summaries, err := s.repo.ListSessionSummaries(req.SessionID, 8)
	if err != nil {
		return mem.L0AssemblyResult{}, err
	}
	summarizedFrom, summarizedTo := 0, 0
	if seg, from, to, ok := s.buildSummarySegment(summaries); ok {
		segments = append(segments, seg)
		summarizedFrom, summarizedTo = from, to
	}

	tokenWindow := s.resolveTokenWindow(settings.RecentWindowTokens, budget)
	hardCap := s.resolveTurnCap(settings.RecentWindowTurns)

	history, err := s.repo.ListLatestMessagesByTokens(req.SessionID, tokenWindow, hardCap)
	if err != nil {
		return mem.L0AssemblyResult{}, err
	}

	historySegments, recentTurnsCount, recentTokensCount := s.buildHistorySegments(history)
	segments = append(segments, historySegments...)

	userInput := strings.TrimSpace(req.UserMessage)
	if userInput != "" {
		segments = append(segments, mem.L0Segment{
			Section: "user.input",
			Role:    "user",
			Source:  "messages[current]",
			Tokens:  estimateTokensApprox(userInput),
			Content: userInput,
			Preview: previewText(userInput, l0PreviewLimit),
		})
	}

	totalTokens := sumSegmentTokens(segments)
	warningCodes := []string{}
	truncatedCount := 0
	truncateStrategy := settings.TruncateStrategy
	if totalTokens > budget {
		segments, truncatedCount, warningCodes, truncateStrategy = applyTruncation(segments, budget, settings.TruncateStrategy)
		totalTokens = sumSegmentTokens(segments)
	}

	usedRatio := 0.0
	if contextWindow > 0 {
		usedRatio = float64(totalTokens) / float64(contextWindow)
		if math.IsInf(usedRatio, 0) || math.IsNaN(usedRatio) {
			usedRatio = 0
		}
		if usedRatio > 1 {
			warningCodes = appendUnique(warningCodes, "exceeded")
			usedRatio = 1
		} else if usedRatio >= 0.95 {
			warningCodes = appendUnique(warningCodes, "near_limit")
		}
	}

	promptMessages := assembleChatMessages(segments)

	result := mem.L0AssemblyResult{
		Segments:              redactSegments(segments, previewMode),
		PromptMessages:        promptMessages,
		BudgetTokens:          budget,
		PromptTokenEstimate:   totalTokens,
		UsedRatioEstimate:     usedRatio,
		RecentWindowTurns:     recentTurnsCount,
		RecentWindowTokens:    recentTokensCount,
		SummarizedTurnFrom:    summarizedFrom,
		SummarizedTurnTo:      summarizedTo,
		TruncateStrategy:      truncateStrategy,
		TruncatedMessageCount: truncatedCount,
		WarningCodes:          warningCodes,
	}

	if previewMode {
		result.PromptMessages = redactPromptMessages(result.PromptMessages)
		return result, nil
	}

	if shouldWriteSnapshot(settings.SnapshotMode, usedRatio, len(warningCodes) > 0) {
		snap := buildSnapshot(req, settings, result, segments, contextWindow, summarizedFrom, summarizedTo)
		if err := s.repo.InsertL0AssemblySnapshot(snap); err == nil {
			result.SnapshotID = snap.ID
		}
	}

	return result, nil
}

// resolveL0Settings 读取智能体级设置；未配置或无行时回退服务默认。
func (s *MemoryL0Service) resolveL0Settings(agentID string) mem.L0Settings {
	defaults := mem.L0Settings{
		RecentWindowTurns:  12,
		RecentWindowTokens: 0,
		SummaryThreshold:   0.6,
		SummaryKeepTurns:   4,
		TruncateStrategy:   "summary",
		InjectL1:           true,
		InjectL3:           true,
		InjectL4:           false,
		L3MaxChunks:        5,
		L4MaxPaths:         3,
		SnapshotMode:       "on_warning",
	}
	if agentID == "" {
		return defaults
	}
	settings, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err != nil {
		return defaults
	}
	out := defaults
	if settings.L0RecentWindowTurns > 0 {
		out.RecentWindowTurns = settings.L0RecentWindowTurns
	}
	if settings.L0RecentWindowTokens > 0 {
		out.RecentWindowTokens = settings.L0RecentWindowTokens
	}
	if settings.L0SummaryThreshold > 0 {
		out.SummaryThreshold = settings.L0SummaryThreshold
	}
	if settings.L0SummaryKeepTurns > 0 {
		out.SummaryKeepTurns = settings.L0SummaryKeepTurns
	}
	if strings.TrimSpace(settings.L0TruncateStrategy) != "" {
		out.TruncateStrategy = settings.L0TruncateStrategy
	}
	out.InjectL1 = settings.L0InjectL1
	out.InjectL3 = settings.L0InjectL3
	out.InjectL4 = settings.L0InjectL4
	if settings.L0L3MaxChunks > 0 {
		out.L3MaxChunks = settings.L0L3MaxChunks
	}
	if settings.L0L4MaxPaths > 0 {
		out.L4MaxPaths = settings.L0L4MaxPaths
	}
	if strings.TrimSpace(settings.L0SnapshotMode) != "" {
		out.SnapshotMode = settings.L0SnapshotMode
	}
	return out
}

func (s *MemoryL0Service) buildSystemSegments(ctx context.Context, agentID string, extra []mem.L0Segment) ([]mem.L0Segment, error) {
	out := make([]mem.L0Segment, 0, 4+len(extra))
	if agentID != "" {
		agent, err := s.repo.GetAgentByID(agentID)
		if err == nil {
			if desc := strings.TrimSpace(agent.AgentDescription); desc != "" {
				out = append(out, mem.L0Segment{
					Section: "system.prompt",
					Role:    "system",
					Source:  "agent.description",
					Tokens:  estimateTokensApprox(desc),
					Content: desc,
					Preview: previewText(desc, l0PreviewLimit),
				})
			}
			files, err := s.repo.ListAgentPromptFiles(agentID)
			if err == nil {
				for _, file := range files {
					body := strings.TrimSpace(file.Body)
					if body == "" {
						continue
					}
					out = append(out, mem.L0Segment{
						Section: "system.prompt_file",
						Role:    "system",
						Source:  "agent_prompt_files:" + file.Name,
						Tokens:  estimateTokensApprox(body),
						Content: body,
						Preview: previewText(body, l0PreviewLimit),
					})
				}
			}
		}
		if s.evolution != nil {
			if body, err := s.evolution.BuildSelfPromptAppend(ctx, agentID); err == nil {
				if body = strings.TrimSpace(body); body != "" {
					out = append(out, mem.L0Segment{
						Section: "system.self_evolution",
						Role:    "system",
						Source:  "agent_evolution",
						Tokens:  estimateTokensApprox(body),
						Content: body,
						Preview: previewText(body, l0PreviewLimit),
					})
				}
			}
		}
	}
	for _, seg := range extra {
		if strings.TrimSpace(seg.Section) == "" {
			seg.Section = "system.extra"
		}
		if strings.TrimSpace(seg.Role) == "" {
			seg.Role = "system"
		}
		if seg.Tokens <= 0 {
			seg.Tokens = estimateTokensApprox(seg.Content)
		}
		if strings.TrimSpace(seg.Preview) == "" {
			seg.Preview = previewText(seg.Content, l0PreviewLimit)
		}
		out = append(out, seg)
	}
	return out, nil
}

// buildL1Segment 委托给已配置的 L1PromptSource。无活动 L1 任务或缺少源时返回 false，保持片段列表干净。
func (s *MemoryL0Service) buildL1Segment(ctx context.Context, sessionID, agentID string) (mem.L0Segment, bool) {
	if s.memoryL1 == nil || sessionID == "" {
		return mem.L0Segment{}, false
	}
	block, ok, err := s.memoryL1.RenderActiveTaskForPrompt(ctx, sessionID, agentID)
	if err != nil || !ok {
		return mem.L0Segment{}, false
	}
	body := strings.TrimSpace(block.Content)
	if body == "" {
		return mem.L0Segment{}, false
	}
	tokens := block.Tokens
	if tokens <= 0 {
		tokens = estimateTokensApprox(body)
	}
	source := strings.TrimSpace(block.Source)
	if source == "" {
		source = "memory.l1"
	}
	return mem.L0Segment{
		Section: "memory.l1",
		Role:    "system",
		Source:  source,
		Tokens:  tokens,
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, true
}

// buildL2Segment 委托给已配置的 L2RecallSource。MemoryL2 自身强制 `l2_recall_enabled` 与 recall_max，此处仅将「无源/无命中」转为 ok=false。
func (s *MemoryL0Service) buildL2Segment(ctx context.Context, sessionID, agentID, query string) (mem.L0Segment, bool) {
	if s.memoryL2 == nil || sessionID == "" {
		return mem.L0Segment{}, false
	}
	return s.memoryL2.RecallSegmentForL0(ctx, sessionID, agentID, query)
}

// buildL3Segment 委托给已配置的 L3RecallSource。MemoryL3 自身强制 `l3_enabled`、作用域、top-k 与每次召回字符预算，此处仅将「无源/无命中」转为 ok=false。
func (s *MemoryL0Service) buildL3Segment(ctx context.Context, scope mem.L0MemoryScopeContext) (mem.L0Segment, bool) {
	if s.memoryL3 == nil {
		return mem.L0Segment{}, false
	}
	return s.memoryL3.RecallSegmentForL0WithContext(ctx, scope)
}

// buildL4Segment 委托给已配置的 L4RecallSource。MemoryL4Service 自身强制 `l4_enabled` / `l4_graph_inject_neighbors` 与每智能体邻域预算，此处仅将「无源/无中心」转为 ok=false。
func (s *MemoryL0Service) buildL4Segment(ctx context.Context, scope mem.L0MemoryScopeContext) (mem.L0Segment, bool) {
	if s.memoryL4 == nil {
		return mem.L0Segment{}, false
	}
	return s.memoryL4.NeighborhoodSegmentForL0WithContext(ctx, scope)
}

func (s *MemoryL0Service) memoryScopeContext(req mem.L0AssemblyRequest) mem.L0MemoryScopeContext {
	scope := mem.L0MemoryScopeContext{
		SessionID:   req.SessionID,
		AgentID:     req.AgentID,
		TeamID:      req.TeamID,
		UserID:      req.UserID,
		WorkspaceID: req.WorkspaceID,
		Query:       req.UserMessage,
	}
	if req.SessionID == "" {
		return scope
	}
	session, err := s.repo.GetSessionByID(req.SessionID)
	if err != nil {
		return scope
	}
	if scope.AgentID == "" {
		scope.AgentID = session.AgentID
	}
	if scope.TeamID == "" {
		scope.TeamID = session.TeamID
	}
	return scope
}

func (s *MemoryL0Service) buildSummarySegment(summaries []mem.SessionSummary) (mem.L0Segment, int, int, bool) {
	if len(summaries) == 0 {
		return mem.L0Segment{}, 0, 0, false
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].FromTurn == summaries[j].FromTurn {
			return summaries[i].ToTurn < summaries[j].ToTurn
		}
		return summaries[i].FromTurn < summaries[j].FromTurn
	})
	var b strings.Builder
	from, to := summaries[0].FromTurn, summaries[0].ToTurn
	for i, s := range summaries {
		if i > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "[summary turn %d-%d] %s", s.FromTurn, s.ToTurn, strings.TrimSpace(s.SummaryMarkdown))
		if s.FromTurn < from {
			from = s.FromTurn
		}
		if s.ToTurn > to {
			to = s.ToTurn
		}
	}
	body := b.String()
	return mem.L0Segment{
		Section: "summary",
		Role:    "system",
		Source:  fmt.Sprintf("session_summaries:%d", len(summaries)),
		Tokens:  estimateTokensApprox(body),
		Content: body,
		Preview: previewText(body, l0PreviewLimit),
	}, from, to, true
}

func (s *MemoryL0Service) buildHistorySegments(history []domain.Message) ([]mem.L0Segment, int, int) {
	out := make([]mem.L0Segment, 0, len(history))
	turns := 0
	totalTokens := 0
	for _, msg := range history {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role != "user" && role != "assistant" && role != "tool" && role != "system" {
			continue
		}
		body := strings.TrimSpace(msg.Content)
		if body == "" {
			continue
		}
		tokens := msg.TokenIn + msg.TokenOut
		if tokens <= 0 {
			tokens = estimateTokensApprox(body)
		}
		out = append(out, mem.L0Segment{
			Section: "history",
			Role:    role,
			Source:  "messages:" + msg.ID,
			Tokens:  tokens,
			Content: body,
			Preview: previewText(body, l0PreviewLimit),
		})
		totalTokens += tokens
		if role == "user" {
			turns++
		}
	}
	return out, turns, totalTokens
}

func (s *MemoryL0Service) resolveTokenWindow(configured int, budget int) int {
	if configured > 0 {
		return configured
	}
	if budget <= 0 {
		return 0
	}
	half := budget / 2
	if half < 1024 {
		return 1024
	}
	return half
}

func (s *MemoryL0Service) resolveTurnCap(turns int) int {
	if turns <= 0 {
		return l0HardMessageCap
	}
	return turns * 2 // 每轮 user + assistant
}

// applyTruncation 裁剪片段直至提示落在 `budget` 内。始终保留系统片段、摘要块与 user.input，使模型仍有最新任务。策略控制 `drop_tool_results` / `drop_oldest` / `summary` 的执行顺序。
func applyTruncation(segments []mem.L0Segment, budget int, strategy string) ([]mem.L0Segment, int, []string, string) {
	warnings := []string{"truncated"}
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "drop_tool_results":
		segs, dropped := dropMatching(segments, func(seg mem.L0Segment) bool {
			return seg.Section == "history" && seg.Role == "tool"
		}, budget)
		return segs, dropped, warnings, "drop_tool_results"
	case "drop_oldest":
		segs, dropped := dropOldestHistory(segments, budget)
		return segs, dropped, warnings, "drop_oldest"
	case "hybrid":
		segs, dropped1 := dropMatching(segments, func(seg mem.L0Segment) bool {
			return seg.Section == "history" && seg.Role == "tool"
		}, budget)
		if sumSegmentTokens(segs) <= budget {
			return segs, dropped1, warnings, "hybrid"
		}
		segs, dropped2 := dropOldestHistory(segs, budget)
		return segs, dropped1 + dropped2, warnings, "hybrid"
	default: // 无活跃 SummaryService 时 summary 回退 → drop_oldest
		segs, dropped := dropOldestHistory(segments, budget)
		return segs, dropped, append(warnings, "summary_unavailable"), "drop_oldest"
	}
}

func dropMatching(segments []mem.L0Segment, match func(mem.L0Segment) bool, budget int) ([]mem.L0Segment, int) {
	if sumSegmentTokens(segments) <= budget {
		return segments, 0
	}
	dropped := 0
	out := make([]mem.L0Segment, 0, len(segments))
	for _, seg := range segments {
		if match(seg) && sumSegmentTokens(out)+remainingTokens(out, segments)-seg.Tokens >= 0 && sumSegmentTokens(append(append([]mem.L0Segment{}, out...), seg)) > budget {
			dropped++
			continue
		}
		out = append(out, seg)
	}
	return out, dropped
}

// remainingTokens 供 dropMatching 使用，避免过度裁剪。
func remainingTokens(out []mem.L0Segment, all []mem.L0Segment) int {
	if len(out) >= len(all) {
		return 0
	}
	return sumSegmentTokens(all[len(out):])
}

func dropOldestHistory(segments []mem.L0Segment, budget int) ([]mem.L0Segment, int) {
	if sumSegmentTokens(segments) <= budget {
		return segments, 0
	}
	type indexed struct {
		i   int
		seg mem.L0Segment
	}
	historyIdx := []indexed{}
	for i, seg := range segments {
		if seg.Section == "history" {
			historyIdx = append(historyIdx, indexed{i, seg})
		}
	}
	dropped := 0
	keep := make(map[int]bool, len(segments))
	for i := range segments {
		keep[i] = true
	}
	for _, h := range historyIdx {
		out := materialize(segments, keep)
		if sumSegmentTokens(out) <= budget {
			break
		}
		keep[h.i] = false
		dropped++
	}
	return materialize(segments, keep), dropped
}

func materialize(segments []mem.L0Segment, keep map[int]bool) []mem.L0Segment {
	out := make([]mem.L0Segment, 0, len(segments))
	for i, seg := range segments {
		if keep[i] {
			out = append(out, seg)
		}
	}
	return out
}

func sumSegmentTokens(segments []mem.L0Segment) int {
	total := 0
	for _, seg := range segments {
		total += seg.Tokens
	}
	return total
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

// assembleChatMessages 将片段展平为可送模型的 []L0ChatMessage。所有 system/summary/l1/l3/l4 片段合并为顶部单条 system，以兼容限制 system 条数的提供方。history 与 user.input 保持角色 1:1。
func assembleChatMessages(segments []mem.L0Segment) []mem.L0ChatMessage {
	var systemBlocks []string
	var rest []mem.L0ChatMessage
	for _, seg := range segments {
		switch seg.Section {
		case "history":
			rest = append(rest, mem.L0ChatMessage{Role: seg.Role, Content: seg.Content})
		case "user.input":
			rest = append(rest, mem.L0ChatMessage{Role: "user", Content: seg.Content})
		default:
			if strings.TrimSpace(seg.Content) == "" {
				continue
			}
			systemBlocks = append(systemBlocks, seg.Content)
		}
	}
	out := make([]mem.L0ChatMessage, 0, len(rest)+1)
	if len(systemBlocks) > 0 {
		out = append(out, mem.L0ChatMessage{
			Role:    "system",
			Content: strings.Join(systemBlocks, "\n\n"),
		})
	}
	return append(out, rest...)
}

func redactSegments(segments []mem.L0Segment, redact bool) []mem.L0Segment {
	if !redact {
		return segments
	}
	out := make([]mem.L0Segment, len(segments))
	for i, seg := range segments {
		seg.Content = ""
		out[i] = seg
	}
	return out
}

func redactPromptMessages(messages []mem.L0ChatMessage) []mem.L0ChatMessage {
	out := make([]mem.L0ChatMessage, len(messages))
	for i, m := range messages {
		m.Content = previewText(m.Content, l0PreviewLimit)
		out[i] = m
	}
	return out
}

func shouldWriteSnapshot(mode string, usedRatio float64, hasWarning bool) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "always":
		return true
	case "off":
		return false
	default: // on_warning
		return hasWarning || usedRatio >= 0.6
	}
}

func buildSnapshot(req mem.L0AssemblyRequest, settings mem.L0Settings, result mem.L0AssemblyResult, segments []mem.L0Segment, contextWindow int, summarizedFrom int, summarizedTo int) mem.L0AssemblySnapshot {
	segmentsJSON := mustMarshalSegments(segments)
	warningJSON := mustMarshalStrings(result.WarningCodes)
	return mem.L0AssemblySnapshot{
		ID:                    newID(),
		SessionID:             req.SessionID,
		RunID:                 req.RunID,
		TurnID:                req.TurnID,
		SpanID:                req.SpanID,
		AgentID:               req.AgentID,
		TeamID:                req.TeamID,
		Provider:              req.Provider,
		Model:                 req.Model,
		ContextWindowTokens:   contextWindow,
		BudgetTokens:          result.BudgetTokens,
		RecentWindowTurns:     result.RecentWindowTurns,
		RecentWindowTokens:    result.RecentWindowTokens,
		SummaryTokenEstimate:  segmentTokensBySection(segments, "summary"),
		L1FieldCount:          countSegments(segments, "memory.l1"),
		L1TokenEstimate:       segmentTokensBySection(segments, "memory.l1"),
		L3ChunkCount:          countSegments(segments, "memory.l3"),
		L3TokenEstimate:       segmentTokensBySection(segments, "memory.l3"),
		L4PathCount:           countSegments(segments, "memory.l4"),
		L4TokenEstimate:       segmentTokensBySection(segments, "memory.l4"),
		PromptTokenEstimate:   result.PromptTokenEstimate,
		PromptTokenActual:     0,
		UsedRatio:             result.UsedRatioEstimate,
		TruncateStrategy:      result.TruncateStrategy,
		TruncatedMessageCount: result.TruncatedMessageCount,
		SummarizedTurnFrom:    summarizedFrom,
		SummarizedTurnTo:      summarizedTo,
		SegmentsJSON:          segmentsJSON,
		WarningCodesJSON:      warningJSON,
		MetadataJSON:          mustMarshalMetadata(settings, req),
		CreatedAt:             nowUTC(),
	}
}

func segmentTokensBySection(segments []mem.L0Segment, section string) int {
	total := 0
	for _, seg := range segments {
		if seg.Section == section {
			total += seg.Tokens
		}
	}
	return total
}

func countSegments(segments []mem.L0Segment, section string) int {
	count := 0
	for _, seg := range segments {
		if seg.Section == section {
			count++
		}
	}
	return count
}

// mustMarshalSegments 仅存储 preview + 元数据，避免完整提示泄漏到快照表。完整内容仍可通过源 `messages.id` 引用访问。
func mustMarshalSegments(segments []mem.L0Segment) string {
	out := make([]map[string]any, len(segments))
	for i, seg := range segments {
		out[i] = map[string]any{
			"section": seg.Section,
			"role":    seg.Role,
			"source":  seg.Source,
			"tokens":  seg.Tokens,
			"preview": seg.Preview,
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func mustMarshalStrings(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	data, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func mustMarshalMetadata(settings mem.L0Settings, req mem.L0AssemblyRequest) string {
	data, err := json.Marshal(map[string]any{
		"settings":            settings,
		"reserved_for_output": req.ReservedForOutput,
	})
	if err != nil {
		return "{}"
	}
	return string(data)
}

func estimateTokensApprox(text string) int {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0
	}
	tokens := len([]rune(trimmed)) / 4
	if tokens < 1 {
		return 1
	}
	return tokens
}
