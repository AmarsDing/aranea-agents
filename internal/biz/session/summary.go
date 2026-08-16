package session

import (
	"context"
	"fmt"
	"strings"
)

// SessionSummary is one persisted rolling-summary row (session_summaries).
type SessionSummary struct {
	ID              string
	SessionID       string
	SummaryMarkdown string
	// TaskStateJSON 是压缩产物双段化的结构化段（v4 压缩契约）：
	// TaskState 的 JSON 序列化，空串 = 本次压缩未产出任务状态。
	TaskStateJSON string
	FromTurn      int
	ToTurn        int
	TokenEstimate int
	CreatedAt     string
}

// TaskState 是跨会话长任务的结构化进度快照（任务状态表层），
// 由压缩 LLM 在叙事摘要之外产出（叙事管"聊了什么"，TaskState 管"做到哪了"）。
// 注入形态与 L1 task_board 渲染保持一致。
type TaskState struct {
	Status   string   `json:"status,omitempty"`
	Done     []string `json:"done,omitempty"`
	Next     string   `json:"next,omitempty"`
	Blockers []string `json:"blockers,omitempty"`
}

const (
	// 防御上限：LLM 失控输出时任务状态注入不得撑爆 prompt。
	taskStateMaxItems     = 8   // done/blockers 各自条目上限
	taskStateMaxItemRunes = 160 // 单条目 rune 上限
	taskStateMaxLineRunes = 200 // status/next 单行上限
)

// Empty 报告状态是否全空（全空不持久化、不注入）。
func (s *TaskState) Empty() bool {
	return s == nil || (s.Status == "" && s.Next == "" && len(s.Done) == 0 && len(s.Blockers) == 0)
}

// Normalize 应用防御上限并清理空白条目。
func (s *TaskState) Normalize() {
	if s == nil {
		return
	}
	s.Status = truncateTaskStateRunes(s.Status, taskStateMaxLineRunes)
	s.Next = truncateTaskStateRunes(s.Next, taskStateMaxLineRunes)
	s.Done = cleanTaskStateList(s.Done)
	s.Blockers = cleanTaskStateList(s.Blockers)
}

// RenderBlock 渲染为 prompt 注入段落（含标题行）。
func (s *TaskState) RenderBlock() string {
	return s.RenderBlockAsOf(0)
}

// RenderBlockAsOf 渲染注入段落；asOfTurn > 0 时标题带时点标注
// （"as of turn N"），让模型能识别状态的新鲜度——非 absorb 压缩路径
// 注入的可能是上一次 LLM 压缩产出的状态，无时点标注会误导模型当作最新进度。
func (s *TaskState) RenderBlockAsOf(asOfTurn int) string {
	if s.Empty() {
		return ""
	}
	var b strings.Builder
	if asOfTurn > 0 {
		fmt.Fprintf(&b, "## Task progress (structured state, as of turn %d)\n", asOfTurn)
	} else {
		b.WriteString("## Task progress (structured state)\n")
	}
	b.WriteString(s.RenderBody())
	return strings.TrimSpace(b.String())
}

// RenderBody 渲染状态正文（Status/Progress/Next/Blockers 行，无标题）。
func (s *TaskState) RenderBody() string {
	if s.Empty() {
		return ""
	}
	var b strings.Builder
	if s.Status != "" {
		b.WriteString("Status: " + s.Status + "\n")
	}
	if len(s.Done) > 0 {
		b.WriteString("Progress:\n")
		for _, d := range s.Done {
			b.WriteString("- " + d + "\n")
		}
	}
	if s.Next != "" {
		b.WriteString("Next: " + s.Next + "\n")
	}
	if len(s.Blockers) > 0 {
		b.WriteString("Blockers:\n")
		for _, bl := range s.Blockers {
			b.WriteString("- " + bl + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func cleanTaskStateList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = truncateTaskStateRunes(item, taskStateMaxItemRunes)
		if item == "" {
			continue
		}
		out = append(out, item)
		if len(out) >= taskStateMaxItems {
			break
		}
	}
	return out
}

func truncateTaskStateRunes(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
}

// StateDelta represents a key-value state mutation (mirrors biz.DomainStateDelta).
type StateDelta struct {
	Operation string
	Path      string
	ValueJSON string
}

// UpdateSessionContextFromLLMUsage delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateSessionContextFromLLMUsage(ctx context.Context, sessionID string, promptTokens, completionTokens, contextWindow int) error {
	return uc.compressionUsecase.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTokens, completionTokens, contextWindow)
}

// UpdateSessionContextAfterCompression delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateSessionContextAfterCompression(ctx context.Context, sessionID string, estimatedPromptTokens int, contextWindow int) error {
	return uc.compressionUsecase.UpdateSessionContextAfterCompression(ctx, sessionID, estimatedPromptTokens, contextWindow)
}

// InsertSessionSummary delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) InsertSessionSummary(ctx context.Context, row SessionSummary) error {
	if strings.TrimSpace(row.SessionID) == "" {
		return validationErr("session id is required")
	}
	return uc.compressionUsecase.InsertSessionSummary(ctx, row)
}

// DeleteSessionSummaries delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) DeleteSessionSummaries(ctx context.Context, sessionID string) error {
	return uc.compressionUsecase.DeleteSessionSummaries(ctx, sessionID)
}

// MaxSessionSummaryToTurn delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	return uc.compressionUsecase.MaxSessionSummaryToTurn(ctx, sessionID)
}

// ListSessionSummaries delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error) {
	return uc.compressionUsecase.ListSessionSummaries(ctx, sessionID)
}

// LatestSessionSummaryTime delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error) {
	return uc.compressionUsecase.LatestSessionSummaryTime(ctx, sessionID)
}

// UpdateSessionListSummary delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error {
	return uc.compressionUsecase.UpdateSessionListSummary(ctx, sessionID, summary)
}

// TryIncrementCompressVersion delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	return uc.compressionUsecase.TryIncrementCompressVersion(ctx, sessionID)
}

// CompressSessionInTx delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error {
	return uc.compressionUsecase.CompressSessionInTx(ctx, sessionID, fn)
}

// SessionSummaryExists delegates to SessionCompressionUsecase (Facade pattern).
func (uc *SessionUsecase) SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error) {
	return uc.compressionUsecase.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
}
