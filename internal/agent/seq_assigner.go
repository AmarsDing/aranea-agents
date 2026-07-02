package agent

import (
	"sync"
	"sync/atomic"
)

// SeqAssigner 为每个 spirit session 维护一个 atomic counter，分配全局单调递增的 Seq。
// 同一 spirit session 内的所有 entity（Task/Turn/Step/TeamStage/TeamRun/MemberSession/PlanBoard/PlanStep）
// 共享同一 counter，保证跨 entity 类型的 Seq 排序正确。
//
// 重启恢复：调用 RestoreFromDB 从 DB 查询 MAX(seq) WHERE spirit_session_id = ? 恢复。
//
// 使用 sync.Map 而非 map+mutex 的理由：
// - LoadOrStore 避免双检锁样板
// - 读多写少（同一 session 的多次 NextSeq 是读 counter 指针）
// - 跨 session 并发无锁
// 注：sync.Map = 1，符合 AS-COG-01（单 struct 内 sync.Map 数 ≤ 1）
//
// 例外说明：本 struct 持有 sync.Map，本身就是单一职责的「Seq 分配管理器」，
// 不是业务 struct，不触发 sync.Map 提取子管理器要求。
type SeqAssigner struct {
	counters sync.Map // sessionID → *atomic.Int64
}

// NewSeqAssigner 创建 SeqAssigner。
func NewSeqAssigner() *SeqAssigner {
	return &SeqAssigner{}
}

// NextSeq 返回 spirit session 的下一个 Seq（从 1 开始）。
func (s *SeqAssigner) NextSeq(spiritSessionID string) int64 {
	if spiritSessionID == "" {
		// 退化场景：不应发生，但兜底，避免空 key 导致全局 counter 污染
		spiritSessionID = "_default_"
	}
	v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
	return v.(*atomic.Int64).Add(1)
}

// RestoreFromDB 从 DB 恢复 Seq 计数器。
// 调用方在启动时查询 MAX(seq) WHERE spirit_session_id = ? 并传入。
// 若已存在的 counter 值大于传入值，不降低（防止并发场景下的回退）。
func (s *SeqAssigner) RestoreFromDB(spiritSessionID string, maxSeqFromDB int64) {
	if spiritSessionID == "" {
		return
	}
	v, _ := s.counters.LoadOrStore(spiritSessionID, &atomic.Int64{})
	current := v.(*atomic.Int64).Load()
	for maxSeqFromDB > current {
		if v.(*atomic.Int64).CompareAndSwap(current, maxSeqFromDB) {
			return
		}
		current = v.(*atomic.Int64).Load()
	}
}
