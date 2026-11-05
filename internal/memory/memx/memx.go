// Package memx 承载应用层对上游 trpc-agent-go memory 包的扩展类型。
//
// 背景：原 vendored fork（pkg/trpc-agent-go）在 memory.go 上有 +55 行私有
// 扩展，出库（切换上游依赖）后这些扩展落在应用层：
//   - Entry/Memory 增加 bi-temporal 有效期（ValidFrom/ValidUntil，P3-8）；
//   - Entry 增加 A-MEM 链接元数据（Links/Keywords/Tags，P3-12）；
//   - SearchOptions 增加 IncludeInvalidated（历史重建查询）；
//   - Service 增加 ProactiveRecall + ConversationContext（P3-11 主动召回）。
//
// 上游 memory.Service 不感知这些扩展。应用内部（internal/memory/...）一律
// 使用 memx 类型；进入框架的边界（runner.WithMemoryService 等）用
// AsUpstream 收敛为上游接口，扩展字段在该方向被丢弃（框架只读基础字段）。
package memx

import (
	"context"
	"time"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
	"trpc.group/trpc-go/trpc-agent-go/session"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// 基础类型直接别名上游，保持与框架签名兼容。
type (
	UserKey      = trpcmemory.UserKey
	Key          = trpcmemory.Key
	Kind         = trpcmemory.Kind
	Metadata     = trpcmemory.Metadata
	AddOption    = trpcmemory.AddOption
	UpdateOption = trpcmemory.UpdateOption
	UpdateResult = trpcmemory.UpdateResult
	ToolCreator  = trpcmemory.ToolCreator
)

const (
	KindFact    = trpcmemory.KindFact
	KindEpisode = trpcmemory.KindEpisode
)

var (
	WithMetadata       = trpcmemory.WithMetadata
	WithUpdateMetadata = trpcmemory.WithUpdateMetadata
	WithUpdateResult   = trpcmemory.WithUpdateResult
	ResolveAddOptions  = trpcmemory.ResolveAddOptions
	// ResolveUpdateOptions / ResolveUpdateResult 直接复用上游：
	// UpdateOption 集合与上游一致，无扩展。
	ResolveUpdateOptions = trpcmemory.ResolveUpdateOptions
	ResolveUpdateResult  = trpcmemory.ResolveUpdateResult
)

// Memory 平铺镜像上游 memory.Memory 全部字段并扩展 bi-temporal 有效期。
// 不采用嵌入是为了保持 e.Memory.Memory（文本）等访问路径与 fork 一致。
type Memory struct {
	Memory      string     `json:"memory"`                 // Memory content.
	Topics      []string   `json:"topics,omitempty"`       // Memory topics (array).
	LastUpdated *time.Time `json:"last_updated,omitempty"` // Last update time.

	// Episodic memory fields.
	Kind         Kind       `json:"kind,omitempty"`         // Memory kind: "fact" or "episode".
	EventTime    *time.Time `json:"event_time,omitempty"`   // When the event occurred.
	Participants []string   `json:"participants,omitempty"` // People involved in the event.
	Location     string     `json:"location,omitempty"`     // Where the event took place.

	// Bi-temporal validity (P3-8). nil ValidUntil 表示当前有效。
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// toUpstream 收敛为上游 Memory：丢弃扩展字段。
func (m *Memory) toUpstream() *trpcmemory.Memory {
	if m == nil {
		return nil
	}
	return &trpcmemory.Memory{
		Memory:       m.Memory,
		Topics:       m.Topics,
		LastUpdated:  m.LastUpdated,
		Kind:         m.Kind,
		EventTime:    m.EventTime,
		Participants: m.Participants,
		Location:     m.Location,
	}
}

// Entry 在上游 memory.Entry 基础上扩展有效期与链接元数据。
// Memory 字段遮蔽上游同名字段以携带 Memory 级有效期。
type Entry struct {
	trpcmemory.Entry
	Memory *Memory `json:"memory"`

	// Bi-temporal validity (P3-8)，镜像 Memory.ValidFrom/ValidUntil。
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`

	// Links/Keywords/Tags (P3-12)：A-MEM 风格记忆链接与演化元数据。
	Links    []string `json:"links,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

// ToUpstream 收敛为上游 Entry：丢弃扩展字段（有效期/链接元数据）。
func (e *Entry) ToUpstream() *trpcmemory.Entry {
	if e == nil {
		return nil
	}
	up := e.Entry
	if e.Memory != nil {
		up.Memory = e.Memory.toUpstream()
	}
	return &up
}

// EntriesToUpstream 批量转换。
func EntriesToUpstream(entries []*Entry) []*trpcmemory.Entry {
	if entries == nil {
		return nil
	}
	out := make([]*trpcmemory.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ToUpstream())
	}
	return out
}

// SearchOptions 在上游基础上扩展 IncludeInvalidated。
type SearchOptions struct {
	trpcmemory.SearchOptions
	// IncludeInvalidated 纳入 ValidUntil 已过期的记忆（默认 false）。
	IncludeInvalidated bool
}

// SearchOption configures optional parameters for SearchMemories.
type SearchOption func(*SearchOptions)

// WithSearchOptions overrides the SearchOptions wholesale.
func WithSearchOptions(s SearchOptions) SearchOption {
	return func(o *SearchOptions) { *o = s }
}

// WithBaseOptions 以上游 SearchOptions 为基底（框架方向调用适配用）。
func WithBaseOptions(b trpcmemory.SearchOptions) SearchOption {
	return func(o *SearchOptions) { o.SearchOptions = b }
}

// ResolveSearchOptions applies SearchOption funcs on top of a base
// SearchOptions that already contains the query string.
func ResolveSearchOptions(query string, opts []SearchOption) SearchOptions {
	o := SearchOptions{}
	o.Query = query
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

// ConversationContext captures the current conversation state for
// proactive recall. Fields are optional; empty values are ignored.
type ConversationContext struct {
	// MentionedEntities are people, places, or topics mentioned in the
	// conversation. Each entity is used as a search keyword to retrieve
	// related memories.
	MentionedEntities []string
	// CurrentTopic is the topic of the current conversation turn.
	CurrentTopic string
	// UserStatement is the user's latest statement, used for contradiction
	// detection: conflicting stored memories are prioritised.
	UserStatement string
}

// Service 是应用层记忆服务接口：上游 memory.Service 的全套方法
// （Entry 换成 memx.Entry）+ ProactiveRecall 主动召回。
type Service interface {
	ReadMemories(ctx context.Context, userKey UserKey, limit int) ([]*Entry, error)
	SearchMemories(ctx context.Context, userKey UserKey, query string, opts ...SearchOption) ([]*Entry, error)
	AddMemory(ctx context.Context, userKey UserKey, memory string, topics []string, opts ...AddOption) error
	UpdateMemory(ctx context.Context, memoryKey Key, memory string, topics []string, opts ...UpdateOption) error
	DeleteMemory(ctx context.Context, memoryKey Key) error
	ClearMemories(ctx context.Context, userKey UserKey) error
	Tools() []tool.Tool
	EnqueueAutoMemoryJob(ctx context.Context, sess *session.Session) error
	// ProactiveRecall 基于当前对话上下文主动召回关联记忆（P3-11）。
	// 空上下文 → 空列表；默认过滤已失效记忆；矛盾检测优先。
	ProactiveRecall(ctx context.Context, userKey UserKey, convCtx ConversationContext) ([]*Entry, error)
	Close() error
}

// upstreamShim 把 memx.Service 收敛为上游 memory.Service。
type upstreamShim struct {
	inner Service
}

var _ trpcmemory.Service = (*upstreamShim)(nil)

// AsUpstream 把应用层 memx.Service 适配为上游 memory.Service，
// 供框架边界（runner.WithMemoryService 等）使用。nil → nil。
func AsUpstream(svc Service) trpcmemory.Service {
	if svc == nil {
		return nil
	}
	return &upstreamShim{inner: svc}
}

func (s *upstreamShim) ReadMemories(ctx context.Context, uk trpcmemory.UserKey, limit int) ([]*trpcmemory.Entry, error) {
	entries, err := s.inner.ReadMemories(ctx, uk, limit)
	if err != nil {
		return nil, err
	}
	return EntriesToUpstream(entries), nil
}

func (s *upstreamShim) SearchMemories(ctx context.Context, uk trpcmemory.UserKey, query string, opts ...trpcmemory.SearchOption) ([]*trpcmemory.Entry, error) {
	base := trpcmemory.ResolveSearchOptions(query, opts)
	entries, err := s.inner.SearchMemories(ctx, uk, query, WithBaseOptions(base))
	if err != nil {
		return nil, err
	}
	return EntriesToUpstream(entries), nil
}

func (s *upstreamShim) AddMemory(ctx context.Context, uk trpcmemory.UserKey, mem string, topics []string, opts ...trpcmemory.AddOption) error {
	return s.inner.AddMemory(ctx, uk, mem, topics, opts...)
}

func (s *upstreamShim) UpdateMemory(ctx context.Context, mk trpcmemory.Key, mem string, topics []string, opts ...trpcmemory.UpdateOption) error {
	return s.inner.UpdateMemory(ctx, mk, mem, topics, opts...)
}

func (s *upstreamShim) DeleteMemory(ctx context.Context, mk trpcmemory.Key) error {
	return s.inner.DeleteMemory(ctx, mk)
}

func (s *upstreamShim) ClearMemories(ctx context.Context, uk trpcmemory.UserKey) error {
	return s.inner.ClearMemories(ctx, uk)
}

func (s *upstreamShim) Tools() []tool.Tool {
	return s.inner.Tools()
}

func (s *upstreamShim) EnqueueAutoMemoryJob(ctx context.Context, sess *session.Session) error {
	return s.inner.EnqueueAutoMemoryJob(ctx, sess)
}

func (s *upstreamShim) Close() error {
	return s.inner.Close()
}
