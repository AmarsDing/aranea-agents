package biz

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

// ── P3 M2: Agent Case 经验记忆（EverOS Agent Memory 启发）────────────────
//
// User Memory（L3 facts）理解用户；Agent Case 理解任务。会话结束后由
// AutoMemoryWorker 在用户记忆提取之后追加提取，产出结构化经验
// （goal/approach/outcome/pitfalls/tools_used）落 memory_agent_cases 表，
// 供 M3 召回注入与 M4 case→skill 蒸馏消费。

// Agent Case 结局判定。
const (
	AgentCaseOutcomeSuccess = "success"
	AgentCaseOutcomePartial = "partial"
	AgentCaseOutcomeFailure = "failure"
)

// AgentCase 是一次任务执行沉淀的结构化经验，面向 Agent 复用（非用户画像）。
type AgentCase struct {
	ID              string
	AgentID         string
	UserID          string
	SourceSessionID string
	Goal            string
	Approach        string
	Outcome         string
	OutcomeSummary  string
	Pitfalls        string
	ToolsUsed       []string
	Quality         float64
}

// ErrAgentCaseSkip 表示提取器判定该会话无实质任务经验（闲聊/单轮问答），
// 属正常跳过而非失败——Worker 收到此错误时整条跳过，不走启发式降级。
var ErrAgentCaseSkip = errors.New("biz: no extractable agent case in conversation")

// AgentCaseReader 按会话查询已提取的 Case（幂等守卫 + M3 召回基础）。
// 无记录时返回 (nil, nil)。
// Stability:evolving
type AgentCaseReader interface {
	GetAgentCaseBySession(ctx context.Context, agentID, sessionID string) (*AgentCase, error)
}

// AgentCaseWriter 幂等写入 Case：唯一锚点 (agent_id, source_session_id)，
// 重复提取/重试时覆盖更新而非新增重复行。
// Stability:evolving
type AgentCaseWriter interface {
	UpsertAgentCase(ctx context.Context, c AgentCase) error
}

// AgentCaseExtractor 从会话消息中提取 Agent Case（LLM 实现在 service 层）。
// 返回 ErrAgentCaseSkip 表示会话无提取价值；其他错误由调用方降级启发式。
type AgentCaseExtractor interface {
	ExtractCase(ctx context.Context, in ConsolidateInput) (*AgentCase, error)
}

// 预过滤门槛：少于 2 条 user 消息（单轮问答）或总内容过短的会话不产生
// 可靠经验，直接跳过以省 LLM 成本。
const (
	agentCaseMinUserMessages = 2
	agentCaseMinTotalRunes   = 200
)

// ShouldExtractAgentCase 是提取前的零成本预过滤。
func ShouldExtractAgentCase(msgs []ConsolidateMessage) bool {
	userMsgs, totalRunes := 0, 0
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgs++
		}
		totalRunes += utf8.RuneCountInString(m.Content)
		if userMsgs >= agentCaseMinUserMessages && totalRunes >= agentCaseMinTotalRunes {
			return true
		}
	}
	return false
}

// HeuristicAgentCase 是 LLM 不可用时的保底提取：goal 取首条 user 消息，
// outcome 按会话是否正常收尾（末条为 assistant 回复）判定，tools_used 从
// 工具消息去重收集。approach/pitfalls 留空——启发式无法可靠推断，宁缺毋滥。
// 无 user 消息时返回 nil。
func HeuristicAgentCase(in ConsolidateInput) *AgentCase {
	var goal string
	var tools []string
	seen := map[string]struct{}{}
	for _, m := range in.Messages {
		if goal == "" && m.Role == "user" {
			goal = truncateRunes(strings.TrimSpace(m.Content), 120)
		}
		if name := strings.TrimSpace(m.ToolName); name != "" {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				tools = append(tools, name)
			}
		}
	}
	if goal == "" {
		return nil
	}
	outcome := AgentCaseOutcomePartial
	if n := len(in.Messages); n > 0 && in.Messages[n-1].Role == "assistant" {
		outcome = AgentCaseOutcomeSuccess
	}
	return &AgentCase{
		Goal:      goal,
		Outcome:   outcome,
		ToolsUsed: tools,
		Quality:   ExtractionQualityHeuristic,
	}
}
