// Package skill — Skill ID generator (P0-7 A14)
//
// 替代 internal/data/skill.go 的 fmt.Sprintf("skill_%d", UnixNano())：
// 高并发/时钟回拨下可碰撞无兜底。ULID 依赖不在 go.mod 且本轮禁止新增依赖
// （设计 §11.4），故按任务规格以 crypto/rand 128bit + 十六进制编码自实现，
// 布局沿用 ULID 语义：48-bit 毫秒时间戳段 + 80-bit 随机段（评审注 ④ 要求
// 时间戳段毫秒精度、与 biz.newAgentCatalogID 的 crypto/rand 模式同源）。
//
//	"skill_" + hex(48bit ms timestamp || 80bit crypto/rand) = 38 字符
//
// 仅用于新记录；存量 skill_%d ID 不迁移（字符串主键混排兼容，排序走
// created_at 字段——设计 §8.5④ 反作用缓解）。
package skill

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"sync/atomic"
	"time"
)

// skillIDRandFallback mirrors biz.newAgentCatalogID 的兜底语义：crypto/rand
// 失败（极低概率）时以原子计数器填充随机段，保证不返回重复 ID。
var skillIDRandFallback uint64

// NewSkillID returns a collision-resistant skill ID for NEW rows:
// "skill_" + 32 hex chars (6-byte big-endian ms timestamp + 10-byte random).
func NewSkillID() string {
	buf := make([]byte, 16) // 128bit
	// 48-bit 毫秒时间戳段（ULID 语义）：取 UnixMilli 的低 6 字节大端序。
	var tsb [8]byte
	binary.BigEndian.PutUint64(tsb[:], uint64(time.Now().UnixMilli()))
	copy(buf[:6], tsb[2:])
	if _, err := rand.Read(buf[6:]); err != nil {
		n := atomic.AddUint64(&skillIDRandFallback, 1)
		binary.BigEndian.PutUint64(buf[8:], n)
		binary.BigEndian.PutUint16(buf[6:8], uint16(n>>48))
	}
	return "skill_" + hex.EncodeToString(buf)
}
