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
	MemberSessionStatusPaused    MemberSessionStatus = "paused"
	MemberSessionStatusCompleted MemberSessionStatus = "completed"
	MemberSessionStatusFailed    MemberSessionStatus = "failed"
	MemberSessionStatusSkipped   MemberSessionStatus = "skipped"
)

// 成员会话版本带（2026-07-28 单写者重设计）：Version 是写者权威层级而非
// 任意编号。UpsertMemberSession 的 VersionLT 守卫与前端 activityV2Store
// 守卫依赖同一层级单调递增；每个版本带内只有一个写者，守卫回归幂等去重
// 本职（同事件重放），不再承担跨写者仲裁。
const (
	// MemberSessionVersionCreated 带 1：runner 发布成员 created（running）。
	// 生命周期事实，执行期可达。
	MemberSessionVersionCreated int64 = 1
	// MemberSessionVersionEvidence 带 2（预留）：runner 发布事实性失败/取消
	// （session error 等执行期即可确认的证据）。当前无生产写者；消息生命
	// 周期（成员产出最终文本）禁止落入本带——出文本 ≠ 工作成功。
	MemberSessionVersionEvidence int64 = 2
	// MemberSessionVersionOutcome 终态权威带：service 终态 outcome pass /
	// Mode B finish / 崩溃 recovery 发布成员终态（completed/failed/skipped）。
	// 唯一可宣布成员成功的写者族；完整证据链（中断 session / 失败 step /
	// 交付物门 / 验证门）只在团队终态时刻齐备，因此终态裁决权威最高、必须恒赢。
	//
	// 取哨兵大值（2026-07-29 哨兵化）：生命周期写者（pause/resume）使用
	// Version++ 单调递增，若 outcome 为固定小值（原 V=3），pause→resume
	// 循环可使 running 成员达到相同版本，导致终态事件被 VersionLT 守卫
	// 静默拒绝（终态丢失、成员永久 running）。哨兵值保证任何递增写者
	// 现实中无法到达本带——终态恒赢，且终态之后无写者。
	MemberSessionVersionOutcome int64 = 1 << 40
)

// IsMemberSessionTerminal reports whether the member session status is a
// terminal outcome (completed/failed/skipped). Terminal records carry
// MemberSessionVersionOutcome authority and must not be overwritten by
// lifecycle writers (pause/resume).
func IsMemberSessionTerminal(s MemberSessionStatus) bool {
	switch s {
	case MemberSessionStatusCompleted, MemberSessionStatusFailed, MemberSessionStatusSkipped:
		return true
	}
	return false
}
