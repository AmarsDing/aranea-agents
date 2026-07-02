package biz

import (
	"encoding/json"
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
)

type StepStatus string

const (
	StepStatusPending     StepStatus = "pending"
	StepStatusRunning     StepStatus = "running"
	StepStatusToolRunning StepStatus = "tool_running"
	StepStatusToolBlocked StepStatus = "tool_blocked"
	StepStatusCompleted   StepStatus = "completed"
	StepStatusFailed      StepStatus = "failed"
	StepStatusCancelled   StepStatus = "cancelled"
)
