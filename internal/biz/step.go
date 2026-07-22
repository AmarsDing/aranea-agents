package biz

import (
	"encoding/json"
	"strings"
	"time"
)

// Step 是 turn 内的工作步骤（v2 模型）：thinking/action/reply/notice/confirm/error。
// 替代旧 Activity 模型中按 kind 区分的多种 activity。
type Step struct {
	ID              string
	TurnID          string
	TaskID          string // 冗余，便于按 task 索引
	SessionID       string
	SpiritSessionID string
	Kind            StepKind
	AuthorAgentKey  string // 发起该 step 的 agent key（spec §3.2）
	Seq             int64  // turn 内的序号（1, 2, 3...）
	Version         int64  // 乐观并发版本号（spec §3.3.5 VersionLT）
	Content         string
	Reasoning       string
	ToolName        string
	ToolCallID      string
	ToolArgs        json.RawMessage // 类型安全的 JSON
	ToolResult      json.RawMessage
	ToolDurationMs  int64
	ToolErrorCode   string
	NoticeType      string // kind=notice: notification type (e.g. "model_router", "cost_guard")
	Status          StepStatus
	IsFinal         bool // reply 是否为最终回复
	StartedAt       time.Time
	CompletedAt     *time.Time
}

type StepKind string

const (
	StepKindThinking StepKind = "thinking"
	StepKindAction   StepKind = "action"
	StepKindReply    StepKind = "reply"
	StepKindNotice   StepKind = "notice"
	StepKindConfirm  StepKind = "confirm"
	StepKindError    StepKind = "error"
	StepKindClarify  StepKind = "clarify"
)

type StepStatus string

const (
	StepStatusPending       StepStatus = "pending"
	StepStatusRunning       StepStatus = "running"
	StepStatusToolRunning   StepStatus = "tool_running"
	StepStatusToolBlocked   StepStatus = "tool_blocked"
	StepStatusCompleted     StepStatus = "completed"
	StepStatusFailed        StepStatus = "failed"
	StepStatusCancelled     StepStatus = "cancelled"
	StepStatusAwaitingInput StepStatus = "awaiting_input"
)

// ClarificationMode 澄清问题的作答模式。
type ClarificationMode string

const (
	ClarificationModeSingle ClarificationMode = "single"
	ClarificationModeMulti  ClarificationMode = "multi"
)

// ClarificationQuestion 单个澄清问题（Step kind=clarify 的 Content 信封元素）。
type ClarificationQuestion struct {
	Question    string            `json:"question"`
	Mode        ClarificationMode `json:"mode"`
	Options     []string          `json:"options"`
	Recommended []string          `json:"recommended"`
}

// ClarificationAnswer 用户对单个问题的作答；Selected 为空表示未作答（按推荐执行）。
type ClarificationAnswer struct {
	Selected []string `json:"selected"`
	Other    string   `json:"other,omitempty"`
}

// ClarificationEnvelope 是 Step(kind=clarify).Content 的 JSON 信封。
// 发布时 Answers 为 nil；用户提交后回写。
type ClarificationEnvelope struct {
	Version   int                     `json:"version"`
	Kind      string                  `json:"kind"` // 固定 "clarification"
	Questions []ClarificationQuestion `json:"questions"`
	Answers   []ClarificationAnswer   `json:"answers"`
}

// ClarificationEnvelopeKind 是信封 Kind 字段的固定值。
const ClarificationEnvelopeKind = "clarification"

// BuildClarifiedContext 把问答渲染为注入 LLM 上下文的用户视角文本。
// 未作答的问题按推荐项作答；既无作答也无推荐时标注「无偏好」。
func (e *ClarificationEnvelope) BuildClarifiedContext() string {
	var b strings.Builder
	b.WriteString("需求澄清结果：\n")
	for i, q := range e.Questions {
		b.WriteString("Q: ")
		b.WriteString(q.Question)
		b.WriteString("\nA: ")
		var ans *ClarificationAnswer
		if i < len(e.Answers) {
			ans = &e.Answers[i]
		}
		switch {
		case ans != nil && (len(ans.Selected) > 0 || ans.Other != ""):
			if len(ans.Selected) > 0 {
				b.WriteString(strings.Join(ans.Selected, "、"))
			}
			if ans.Other != "" {
				if len(ans.Selected) > 0 {
					b.WriteString("；")
				}
				b.WriteString(ans.Other)
			}
		case len(q.Recommended) > 0:
			b.WriteString("按推荐：")
			b.WriteString(strings.Join(q.Recommended, "、"))
		default:
			b.WriteString("无偏好")
		}
		b.WriteString("\n")
	}
	return b.String()
}
