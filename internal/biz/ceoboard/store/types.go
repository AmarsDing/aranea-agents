// Package store 定义 ceoboard 事件存储的数据模型(DTO)与端口约定。
//
// 本文件只放与 biz 层共享的纯数据类型;Ent 仓储实现、spool 回填、
// UTC+8 闭日归档与导出在 P2-5 落地。消费侧接口(agg/service 依赖面)
// 遵循「消费者定义接口」惯例,分别定义在 agg 包(P2-4)与 service 层(P2-7),
// P2-5 的仓储实现这些接口。
//
// 移植保真要点(11 条风险点相关):
//   - Event.UID = 平台:原去重键,全局唯一,幂等依据;
//   - Payload 一律 TEXT 保真(jsonb 会重排键序,破坏脱敏/重放/审计);
//   - OccurredAt 零值 = 信封未带时间(查询侧退 ReceivedAt);
//   - EventFilter.AllowedSubjects 是 raw subject 字面值白名单,不解释 NATS 通配符。
package store

import "time"

// Event 是一条平台事件(归一化后)。
type Event struct {
	UID               string    // event_uid = 平台:原去重键(全局唯一,幂等依据)
	Platform          string    // hcf | novatrader | aranea | ...
	Subject           string    // 原始 subject(含版本后缀)
	SubjectKey        string    // 剥离 .vN 版本后缀后的稳定键(版本演进不丢事件)
	EventID           string    // 源事件 id(信封 event_id)
	EntityID          string    // 业务实体 id(可为空)
	OccurredAt        time.Time // 事件发生时间(信封);零值表示信封未带
	ReceivedAt        time.Time // 看板接收时间
	Payload           string    // 原文(新数据完整存储;历史数据由 payload_complete 标记)
	ImportedFromSpool bool      `json:"imported_from_spool,omitempty"`
	PayloadComplete   bool      `json:"payload_complete"`
}

// Delivery 是一条 webhook 投递记录。
type Delivery struct {
	Key      string    // 幂等键
	Source   string    // aranea | ...
	Event    string    // 事件名
	EntityID string    // 实体 id
	At       time.Time // 接收时间
	Digest   string    // sha256 前 16 hex
	Fields   string    // 允许落库字段(JSON,TEXT 保真)
	Status   string    // ok | failed
	Error    string    // 失败原因
}

// DeadLetter 是一条死信。
type DeadLetter struct {
	ID         int64      `json:"id"`
	Kind       string     `json:"kind"`    // webhook-receive | webhook-process | nats-decode
	Source     string     `json:"source"`  // 来源(aranea / hcf / novatrader),重放路由依据
	RefKey     string     `json:"ref_key"` // 关联键(幂等键 / event_uid)
	Payload    string     `json:"payload"` // 原文
	Reason     string     `json:"reason"`  // 失败原因
	FailedAt   time.Time  `json:"failed_at"`
	ReplayedAt *time.Time `json:"replayed_at,omitempty"`
}

// AuditEntry 是一条高危操作审计。
// 截断口径(写库前 biz 层执行):actor 128 / action 64 / target 256 / detail 1024。
type AuditEntry struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	Target string    `json:"target"`
	OK     bool      `json:"ok"`
	Detail string    `json:"detail"`
}

// LatencySample 是一条延迟物化样本(B 侧幂等:同 metric_id+b_uid 重复忽略)。
type LatencySample struct {
	MetricID  string
	BUID      string // B 侧事件 uid(幂等主键之一)
	EntityID  string
	AUID      string
	LatencyMs int64
	At        time.Time // 取 B 事件时间
}

// PairEvent 是 latency 配对候选事件。
type PairEvent struct {
	UID      string
	EntityID string
	At       time.Time // occurred_at;缺失时退 received_at(查询侧归一)
}

// EventCursor 是事件游标分页位置(received_at, event_uid 双键)。
type EventCursor struct {
	ReceivedAt time.Time
	UID        string
}

// EventFilter 限定事件查询。AllowedSubjects 是 raw subject 字面值白名单,
// 不解释 NATS 通配符(服务端权限裁剪,风险点⑦)。
type EventFilter struct {
	Platform        string
	AllowedSubjects []string
	Subject         string
	EventID         string
	WorkflowID      string
	NodeID          string
	From            time.Time
	To              time.Time
	Cursor          *EventCursor
	Limit           int
}
