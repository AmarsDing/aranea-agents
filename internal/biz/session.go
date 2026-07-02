package biz

import "time"

// SpiritSession 是 spirit 会话的根 entity（v2 模型）。
// 替代旧 Activity 模型中靠 spirit_session_id 字段隐式分组的做法。
//
// 命名说明：本类型命名为 SpiritSession 而非 Session，因为 biz 包已通过
// session_reexport.go 将 Session 作为 session.Session 的类型别名导出。
// SpiritSession 与 SpiritSessionID 字段命名保持一致。
type SpiritSession struct {
	ID            string
	UserID        string
	SpiritAgentID string
	Status        SpiritSessionStatus
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SpiritSessionStatus string

const (
	SpiritSessionStatusActive    SpiritSessionStatus = "active"
	SpiritSessionStatusCompleted SpiritSessionStatus = "completed"
	SpiritSessionStatusFailed    SpiritSessionStatus = "failed"
)
