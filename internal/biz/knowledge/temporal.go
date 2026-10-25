package knowledge

import (
	"context"
	"time"
)

// ── G-05-asof（ADR-KN-20261020-J）：双时态 as-of 查询端口 ─────────────────
//
// TemporalSpan 携带两轴时点，四象限语义：
//
//	T=0, R=0 → current 快报（既有原函数路径，谓词文本一字不改）
//	T≠0, R=0 → T-only：彼时有效、以当前已知信息
//	T=0, R≠0 → R-only：R 时已知且当时仍有效（valid_to 恒等于关闭时刻兼记录端）
//	T≠0, R≠0 → 双轴交：recorded_at <= R AND ( @>T OR (from<=T AND vt>R) )
//
// 保真边界（ADR-J 已知失真四类）：仅边图存在性/link_type/relation/端点有效性
// 保真；weight_f/context 等属性返回当前值；co_activated 与 closed-on-arrival
// 空区间在任何 as-of 象限不可见。检索侧热度副作用（access_log/base-level/
// Hebbian）对 span 非零的查询全部关闭（见 adaptive_router 守卫）。

// TemporalSpan 双时态查询窗口。零值 = 关闭时态（current 快报）。
// 值语义：随 SearchQuery 值拷贝全链路自动携带（PathPrefix 先例）。
type TemporalSpan struct {
	// ValidAt：有效时间轴（T 轴）时点。零值 = 不启用 T 轴。
	ValidAt time.Time
	// RecordedAt：记录时间轴（R 轴）时点。零值 = 不启用 R 轴。
	RecordedAt time.Time
}

// Any 任一轴启用即为时态查询（检索热度副作用随之关闭）。
func (s TemporalSpan) Any() bool { return s.ValidOn() || s.RecordedOn() }

// ValidOn T 轴启用。
func (s TemporalSpan) ValidOn() bool { return !s.ValidAt.IsZero() }

// RecordedOn R 轴启用。
func (s TemporalSpan) RecordedOn() bool { return !s.RecordedAt.IsZero() }

// TemporalActiveLinkReader 时态版活跃边读端口（扩散激活逐跳 BFS 数据源）。
// Stability: evolving（AS-STA-01）——as-of 谓词族尚未覆盖全部热读者。
//
// 契约：投影与 ACL 与 ActiveLinkReader.ListActiveLinks 一致；span 零值时必须
// 等价委托原函数（含 co_activated、valid_to IS NULL 原文本/partial index）；
// 非零时按四象限谓词且任何象限排除 co_activated（ADR-J 失真(a)）。
type TemporalActiveLinkReader interface {
	ListActiveLinksAsOf(ctx context.Context, collectionID string, docIDs []string, span TemporalSpan) ([]ActiveLink, error)
}

// TemporalCollectionLinkReader 时态版集合边读端口（图谱 RPC 数据源）。
// Stability: evolving（AS-STA-01）。
//
// 契约：投影/ACL/linkTypes 过滤与 CollectionLinkReader.ListCollectionLinks 一致；
// span 零值时等价委托原函数（现行行为：不做 valid_to 过滤，closed 边照常返回——
// 前端现状红线）；非零时按四象限谓词且排除 co_activated，保持 id 升序。
type TemporalCollectionLinkReader interface {
	ListCollectionLinksAsOf(ctx context.Context, collectionID string, linkTypes []string, span TemporalSpan) ([]Link, error)
}
