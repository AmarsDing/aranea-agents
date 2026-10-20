package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// G-10 写回 outbox（2026-10-19）：AutoMemory 写回团队库失败此前仅 Warn 即永久
// 丢失，违反 AS-EVT-01「Important = 异步持久化」。本文件定义重试队列的 biz 端口
// 与编解码：失败时落 payload，knowledge_writeback 后台 worker 周期重放至
// WriteBackSessionFacts。模式复用 decision_record_outbox（M80 E5）。

const (
	// WriteBackOutboxStatusPending 待重放。
	WriteBackOutboxStatusPending = "pending"
	// WriteBackOutboxStatusPublished 已重放成功（审计语义：确实投递）。
	WriteBackOutboxStatusPublished = "published"
	// WriteBackOutboxStatusDead 重试上限触达或 poison（payload 不可解码）终态；
	// payload/last_error 留存供事后排查。
	WriteBackOutboxStatusDead = "dead"
	// WriteBackOutboxMaxAttempts 重放重试上限；触达即翻 dead（同 decision 口径）。
	WriteBackOutboxMaxAttempts = 10
)

// WriteBackOutboxRow 一行待重放写回批次。Payload 为 codec-stable JSON
// （EncodeWriteBackOutboxPayload 产物）。
type WriteBackOutboxRow struct {
	ID          int64
	BatchKey    string
	WorkspaceID string
	Payload     string
	Status      string
	Attempts    int
	LastError   string
	CreatedAt   time.Time
	PublishedAt *time.Time
}

// WriteBackOutboxRepo 写回重试队列持久化端口。
// Stability: Evolving
type WriteBackOutboxRepo interface {
	// EnqueueWriteBackOutbox 落一行待重放批次（batch_key 幂等：同键重复入队忽略，
	// 同批事实反复失败不堆行）。
	EnqueueWriteBackOutbox(ctx context.Context, batchKey string, in WriteBackInput) error
	// ListPendingWriteBackOutbox 按 created_at 升序取 pending 且 attempts 未触达上限
	// 的行。limit <= 0 默认 100。
	ListPendingWriteBackOutbox(ctx context.Context, limit int) ([]WriteBackOutboxRow, error)
	// MarkWriteBackOutboxAttempt 记录一次失败重放；attempts+1 触达上限时同事务翻 dead。
	MarkWriteBackOutboxAttempt(ctx context.Context, id int64, lastError string) error
	// MarkWriteBackOutboxPublished 标记重放成功。
	MarkWriteBackOutboxPublished(ctx context.Context, ids []int64, publishedAt time.Time) error
	// MarkWriteBackOutboxDead 把 poison 行（payload 不可解码，重试永不会成功）直接
	// 翻 dead 终态——区别于 Published：行从未投递，published 会是审计造假。
	MarkWriteBackOutboxDead(ctx context.Context, ids []int64, lastError string) error
}

// WriteBackBatchKey 构造批次幂等键：wb:{sessionID}:{facts 内容哈希前 16 位}。
// 同会话同批事实（含顺序）重复失败归并一行；不同批次各自成行。
func WriteBackBatchKey(sessionID string, facts []WriteBackFact) string {
	h := sha256.New()
	for _, f := range facts {
		stmt := strings.ToLower(strings.TrimSpace(f.Statement))
		fmt.Fprintf(h, "%s:%s\x00", f.FactKind, stmt)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("wb:%s:%s", strings.TrimSpace(sessionID), sum[:16])
}

// writeBackOutboxFact 是 WriteBackFact 的 codec-stable 线上形态：EntryTitle 为
// 执行期回填字段，重放时由写回路径重新计算，不入队。
type writeBackOutboxFact struct {
	FactID     string   `json:"fact_id"`
	Statement  string   `json:"statement"`
	FactKind   string   `json:"fact_kind"`
	Confidence float64  `json:"confidence"`
	SourceKind string   `json:"source_kind"`
	Tags       []string `json:"tags"`
}

type writeBackOutboxPayload struct {
	Workspace string                `json:"workspace"`
	SessionID string                `json:"session_id"`
	AgentID   string                `json:"agent_id"`
	UserID    string                `json:"user_id"`
	Facts     []writeBackOutboxFact `json:"facts"`
}

// EncodeWriteBackOutboxPayload 把写回批次编码为 outbox payload（codec-stable）。
func EncodeWriteBackOutboxPayload(in WriteBackInput) ([]byte, error) {
	p := writeBackOutboxPayload{
		Workspace: in.Workspace,
		SessionID: in.SessionID,
		AgentID:   in.AgentID,
		UserID:    in.UserID,
		Facts:     make([]writeBackOutboxFact, 0, len(in.Facts)),
	}
	for _, f := range in.Facts {
		p.Facts = append(p.Facts, writeBackOutboxFact{
			FactID:     f.FactID,
			Statement:  f.Statement,
			FactKind:   f.FactKind,
			Confidence: f.Confidence,
			SourceKind: f.SourceKind,
			Tags:       f.Tags,
		})
	}
	return json.Marshal(p)
}

// DecodeWriteBackOutboxPayload 还原写回批次；任何字段缺失/类型错都返回 error
// （poison 行由 worker 翻 dead，绝不半解半投）。
func DecodeWriteBackOutboxPayload(payload []byte) (WriteBackInput, error) {
	var p writeBackOutboxPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return WriteBackInput{}, err
	}
	if strings.TrimSpace(p.SessionID) == "" || len(p.Facts) == 0 {
		return WriteBackInput{}, fmt.Errorf("writeback outbox payload missing session_id/facts")
	}
	in := WriteBackInput{
		Workspace: p.Workspace,
		SessionID: p.SessionID,
		AgentID:   p.AgentID,
		UserID:    p.UserID,
		Facts:     make([]WriteBackFact, 0, len(p.Facts)),
	}
	for _, f := range p.Facts {
		if strings.TrimSpace(f.Statement) == "" {
			return WriteBackInput{}, fmt.Errorf("writeback outbox fact missing statement")
		}
		in.Facts = append(in.Facts, WriteBackFact{
			FactID:     f.FactID,
			Statement:  f.Statement,
			FactKind:   f.FactKind,
			Confidence: f.Confidence,
			SourceKind: f.SourceKind,
			Tags:       f.Tags,
		})
	}
	return in, nil
}
