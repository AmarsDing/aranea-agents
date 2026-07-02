package biz

import "time"

// MemberSession 是 team 内的成员会话（v2 模型）。
// 一个 TeamRun 包含多个 MemberSession（每个 agent 一个）。
// MemberSession 内的 turn 是终止层，不能再嵌套 team_stage（三层硬约束）。
type MemberSession struct {
	ID              string // = SessionActivityID(teamID, agentKey) 的 v2 版本
	TeamRunID       string
	TeamStageID     string
	TaskID          string
	SessionID       string // member 自己的 session ID（用于 lazy load）
	SpiritSessionID string
	AgentKey        string
	AgentName       string
	AvatarURL       string
	Status          MemberSessionStatus
	Seq             int64
	Version         int64      // 乐观并发版本号（spec §3.3.5 VersionLT）
	StartedAt       time.Time  // 成员会话开始时间
	FinishedAt      *time.Time // 成员会话结束时间（nil 表示未结束）
	Error           string     // 失败时的错误信息（空字符串表示无错误）
}

type MemberSessionStatus string

const (
	MemberSessionStatusPending   MemberSessionStatus = "pending"
	MemberSessionStatusRunning   MemberSessionStatus = "running"
	MemberSessionStatusCompleted MemberSessionStatus = "completed"
	MemberSessionStatusFailed    MemberSessionStatus = "failed"
	MemberSessionStatusSkipped   MemberSessionStatus = "skipped"
)
