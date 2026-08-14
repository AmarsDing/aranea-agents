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
	Danger          bool   // kind=confirm: 高危动作标记（75 A5 敏感词命中），前端渲染高危徽标
	Status          StepStatus
	IsFinal         bool // reply 是否为最终回复
	StartedAt       time.Time
	CompletedAt     *time.Time
}

// SynthesisAuthorAgentKey 精灵总结 turn 的 reply step 专属 AuthorAgentKey。
// 所有团队完成后由 TeamStarter 触发 synthesis turn（TurnInput.Synthesis=true），
// 该 turn 的 reply step（即总结报告）以此标记覆盖原 agent key，
// 前端据此渲染「任务总结」徽章高亮（2026-07-27）。
const SynthesisAuthorAgentKey = "spirit-synthesis"

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

// ClarificationResolution 记录澄清的收口方式。
type ClarificationResolution string

const (
	// ClarificationResolutionAutoDefault 全部问题按推荐默认自动作答（假设式前进），
	// turn 未挂起等待用户。空值表示由用户作答（卡片提交/自由回复）。
	ClarificationResolutionAutoDefault ClarificationResolution = "auto_default"
)

// ClarificationAllRecommended 报告全部澄清问题是否都携带推荐默认，
// 是假设式前进（auto_default）的判据之一；零问题返回 false。
func ClarificationAllRecommended(qs []ClarificationQuestion) bool {
	if len(qs) == 0 {
		return false
	}
	for _, q := range qs {
		if len(q.Recommended) == 0 {
			return false
		}
	}
	return true
}

// ClarificationEnvelope 是 Step(kind=clarify).Content 的 JSON 信封。
// 发布时 Answers 为 nil；用户提交后回写。
type ClarificationEnvelope struct {
	Version   int                     `json:"version"`
	Kind      string                  `json:"kind"` // 固定 "clarification"
	Questions []ClarificationQuestion `json:"questions"`
	Answers   []ClarificationAnswer   `json:"answers"`
	// OriginalInput 是触发澄清的原始用户输入，用于服务重启 / 多副本后惰性重建续跑输入。
	OriginalInput string `json:"original_input,omitempty"`
	// IntentArtifactJSON 持久化触发澄清时的意图产物（internal/agent/intent.Artifact
	// 的 JSON 序列化）。进程内 clarification cache 丢失后，续跑时从
	// 信封恢复意图产物，避免为重写后的输入重跑 Intent Pass LLM。
	// biz 层不依赖 agent 层，故以原始 JSON 字符串存储。
	IntentArtifactJSON string `json:"intent_artifact_json,omitempty"`
	// FreeText 是用户在澄清等待态直接发消息产生的自由回复内容。
	FreeText string `json:"free_text,omitempty"`
	// Resolution 记录收口方式：auto_default=按推荐默认自动作答；空=用户作答。
	Resolution ClarificationResolution `json:"resolution,omitempty"`
}

// ClarificationEnvelopeKind 是信封 Kind 字段的固定值。
const ClarificationEnvelopeKind = "clarification"

// ApplyRecommendedAnswers 按各问题的推荐项填充 Answers（假设式前进）；
// 无推荐项的问题保持空作答。已有 Answers 会被覆盖重建。
func (e *ClarificationEnvelope) ApplyRecommendedAnswers() {
	e.Answers = make([]ClarificationAnswer, len(e.Questions))
	for i, q := range e.Questions {
		if len(q.Recommended) > 0 {
			e.Answers[i].Selected = append([]string(nil), q.Recommended...)
		}
	}
}

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
	if e.FreeText != "" {
		b.WriteString("补充说明：")
		b.WriteString(e.FreeText)
		b.WriteString("\n")
	}
	return b.String()
}
