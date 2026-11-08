// Package adapter 实现 ceoboard 数据源适配器层:rest-poll 与 webhook-recv 实接,
// nats-jetstream 消费,aranea 进程内 outbox 直投;sse/ws/db-query 注册但不接线。
// 统一限流(每平台 QPS 池)与失败隔离(工作流×节点×适配器实例粒度)。
//
// 本文件只放跨适配器共享的类型常量与运行态视图(agg 依赖面);
// 各适配器实现(Poller/WebhookReceiver/NATSManager/TokenProvider)在 P2-6 落地。
package adapter

import (
	"regexp"
	"strings"
	"time"
)

// 适配器类型常量。
const (
	KindNATS     = "nats-jetstream"
	KindRestPoll = "rest-poll"
	KindWebhook  = "webhook-recv"
	KindSSE      = "sse"
	KindWS       = "ws"
	KindHybrid   = "hybrid"
	KindDBQuery  = "db-query"
)

// HasRESTSide 报告该适配器是否带 REST 轮询面(hybrid 的 rest 对账半侧)。
func HasRESTSide(kind string) bool {
	return kind == KindRestPoll || kind == KindHybrid
}

// PollState 是单轮询目标的运行态(rest-poll / hybrid 对账侧 / 平台探针)。
type PollState struct {
	Key        string           `json:"key"`
	URL        string           `json:"url"`
	LastOKAt   time.Time        `json:"last_ok_at,omitempty"` // 最后成功时间(陈旧判定依据)
	LastErrAt  time.Time        `json:"last_err_at,omitempty"`
	LastErr    string           `json:"last_err,omitempty"`
	FailCount  int              `json:"fail_count"` // 连续失败次数
	LatencyMs  int64            `json:"latency_ms"`
	AsOf       time.Time        `json:"as_of,omitempty"` // Items 数据的对应时刻
	Items      []map[string]any `json:"-"`               // 最近一次成功响应归一出的列表(供 gauge/ratio)
	RawSnippet string           `json:"raw_snippet,omitempty"`
	OpenUntil  time.Time        `json:"open_until,omitempty"` // 断路器断开至
}

// CircuitOpen 报告断路器是否断开中。
func (s *PollState) CircuitOpen(now time.Time) bool { return now.Before(s.OpenUntil) }

// NATSNodeState 是单节点 NATS 消费运行态。
type NATSNodeState struct {
	NodeKey        string    `json:"node_key"`
	Endpoint       string    `json:"endpoint"`
	Stream         string    `json:"stream"`
	StreamMissing  bool      `json:"stream_missing,omitempty"`  // 声明的流在 JetStream 不存在(漂移/未启用)
	StreamOptional bool      `json:"stream_optional,omitempty"` // 声明显式可选(条件创建流,缺失=未启用非漂移)
	Connected      bool      `json:"connected"`                 // NATS 连接存活
	Consuming      bool      `json:"consuming"`                 // 至少一个 consumer 建立成功
	Backlog        bool      `json:"backlog,omitempty"`         // 最近一批 Fetch 满批(积压指示)
	LastErr        string    `json:"last_err,omitempty"`
	LastMsgAt      time.Time `json:"last_msg_at,omitempty"` // 最近一次收到消息(事件侧"最后成功时间")
	MsgCount       int64     `json:"msg_count"`             // 累计新事件数(去重后)
	DupCount       int64     `json:"dup_count"`             // 重复投递被去重次数
	DecodeErr      int64     `json:"decode_err"`            // 非空消息解码失败进死信次数
	EmptyDrops     int64     `json:"empty_drops"`           // 空 payload 直接 Ack 丢弃次数
	Reconnects     int       `json:"reconnects"`            // 累计重连次数
	StartedAt      time.Time `json:"started_at"`
}

// ConsumerState 是单 durable consumer 的运行态(consumers 列表视图)。
type ConsumerState struct {
	NodeKey     string    `json:"node_key"`
	Platform    string    `json:"platform"`
	Stream      string    `json:"stream"`
	Durable     string    `json:"durable"`
	Subject     string    `json:"subject"`
	NumPending  int       `json:"num_pending"`
	Connected   bool      `json:"connected"`
	AsOf        time.Time `json:"as_of"`
	SampleError string    `json:"sample_error,omitempty"`
}

// WebhookEvent 是一条归一化后的 webhook 投递事件(接收端返回与视图共用)。
type WebhookEvent struct {
	Source     string         `json:"source"`
	Event      string         `json:"event"`
	Key        string         `json:"key"` // 幂等键(idempotency_key / event_id / 实体 id / body 摘要兜底)
	EntityID   string         `json:"entity_id,omitempty"`
	At         time.Time      `json:"at"`
	Digest     string         `json:"digest"` // sha256(body) 前 16 hex
	Fields     map[string]any `json:"fields,omitempty"`
	Deduped    bool           `json:"deduped"` // 本次是否为重复投递(被去重)
	Status     string         `json:"status"`  // ok | failed
	ErrMsg     string         `json:"error,omitempty"`
	PersistErr error          `json:"-"`
}

// versionSuffixRe 匹配 subject 末尾内嵌版本号(.vN)。
var versionSuffixRe = regexp.MustCompile(`\.v\d+$`)

// SubjectKey 剥离版本后缀得到稳定键(mm.intel.normalized.v1 → mm.intel.normalized)。
func SubjectKey(subject string) string { return versionSuffixRe.ReplaceAllString(subject, "") }

// HostPort 从 nats endpoint 取 host:port(剥 scheme 与路径)。
func HostPort(endpoint string) string {
	e := strings.TrimPrefix(endpoint, "nats://")
	e = strings.TrimPrefix(e, "tls://")
	if i := strings.IndexByte(e, '/'); i >= 0 {
		e = e[:i]
	}
	return e
}
