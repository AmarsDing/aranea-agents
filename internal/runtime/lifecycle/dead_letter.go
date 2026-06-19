package lifecycle

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// DeadLetterMessage 是死信队列中的消息。
type DeadLetterMessage struct {
	ID         string        // 唯一 ID（调用方生成，如 UUID）
	Source     string        // 来源（如 "pending-queue"、"sleep-time-job"）
	Original   any           // 原始消息（类型由调用方决定）
	Error      string        // 失败原因
	FailedAt   time.Time     // 失败时间
	RetryCount int           // 已重试次数
	MaxRetries int           // 最大重试次数
}

// RetryHandler 重试处理函数。返回 error 表示重试仍失败。
type RetryHandler func(ctx context.Context, msg *DeadLetterMessage) error

// DeadLetterQueue 是内存死信队列，提供查询/重试/丢弃能力。
//
// 设计目标（A4）：替代 pending queue 失败即丢弃的设计。
// 失败消息入死信队列（内存缓冲），提供管理 API 查询/重试/丢弃，
// 限制队列大小避免无限增长。
//
// 与方案 C 的 DeadLetterRepo 统一设计：DeadLetterQueue 是内存缓冲层，
// 可选持久化到 DB（由调用方决定，通过 PersistHook 实现）。
//
// 并发安全：所有方法均可并发调用。
type DeadLetterQueue struct {
	mu        sync.Mutex
	messages  []DeadLetterMessage
	maxSize   int
	lg        loggateway.Logger
	persistFn func(context.Context, *DeadLetterMessage) error // 可选持久化钩子
}

// NewDeadLetterQueue 创建一个死信队列。maxSize 为 0 表示使用默认值 1000。
func NewDeadLetterQueue(maxSize int, lg loggateway.Logger) *DeadLetterQueue {
	if maxSize <= 0 {
		maxSize = 1000
	}
	if lg != nil {
		lg = lg.With(loggateway.Domain("dead_letter"))
	}
	return &DeadLetterQueue{
		maxSize: maxSize,
		lg:      lg,
	}
}

// SetPersistHook 设置持久化钩子。设置后，Enqueue 会异步调用 hook 持久化消息到 DB。
func (q *DeadLetterQueue) SetPersistHook(fn func(context.Context, *DeadLetterMessage) error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.persistFn = fn
}

// Enqueue 将失败消息入队。若队列已满，丢弃最旧的消息并记录日志。
func (q *DeadLetterQueue) Enqueue(msg DeadLetterMessage) {
	if q == nil {
		return
	}
	if msg.FailedAt.IsZero() {
		msg.FailedAt = time.Now()
	}
	q.mu.Lock()
	// 队列满时丢弃最旧消息
	if len(q.messages) >= q.maxSize {
		if q.lg != nil {
			q.lg.Warn("dead letter queue full, dropping oldest message",
				loggateway.Str("source", msg.Source),
				loggateway.Int("max_size", q.maxSize))
		}
		q.messages = q.messages[1:]
	}
	q.messages = append(q.messages, msg)
	persistFn := q.persistFn
	q.mu.Unlock()

	// 异步持久化（如果配置了 hook）
	if persistFn != nil {
		safego.Go(appctx.Ctx(), "dead_letter.persist", func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := persistFn(ctx, &msg); err != nil && q.lg != nil {
				q.lg.Warn("persist dead letter failed",
					loggateway.Str("source", msg.Source),
					loggateway.Str("id", msg.ID),
					loggateway.Err(err))
			}
		})
	}
}

// List 返回队列中所有消息的副本。支持 limit 限制返回数量（0 表示全部）。
func (q *DeadLetterQueue) List(limit int) []DeadLetterMessage {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if limit <= 0 || limit > len(q.messages) {
		limit = len(q.messages)
	}
	result := make([]DeadLetterMessage, limit)
	copy(result, q.messages[:limit])
	return result
}

// Retry 重试指定 ID 的消息。成功后从队列移除；失败则增加 RetryCount 并保留。
func (q *DeadLetterQueue) Retry(ctx context.Context, id string, handler RetryHandler) error {
	if q == nil || handler == nil {
		return nil
	}
	q.mu.Lock()
	var msg *DeadLetterMessage
	var idx = -1
	for i := range q.messages {
		if q.messages[i].ID == id {
			msg = &q.messages[i]
			idx = i
			break
		}
	}
	if msg == nil {
		q.mu.Unlock()
		return nil
	}
	// 复制一份用于处理（避免在锁内调用 handler）
	msgCopy := *msg
	q.mu.Unlock()

	err := handler(ctx, &msgCopy)
	q.mu.Lock()
	defer q.mu.Unlock()
	// 重新查找（可能在锁外被修改）
	if idx >= len(q.messages) || q.messages[idx].ID != id {
		// 消息已被移除或位置变化，重新查找
		for i := range q.messages {
			if q.messages[i].ID == id {
				idx = i
				break
			}
		}
		if idx >= len(q.messages) || idx < 0 {
			return err
		}
	}
	if err == nil {
		// 成功，从队列移除
		q.messages = append(q.messages[:idx], q.messages[idx+1:]...)
		return nil
	}
	// 失败，增加 RetryCount
	q.messages[idx].RetryCount++
	if q.messages[idx].RetryCount >= q.messages[idx].MaxRetries && q.messages[idx].MaxRetries > 0 {
		// 超过最大重试次数，丢弃
		q.messages = append(q.messages[:idx], q.messages[idx+1:]...)
		if q.lg != nil {
			q.lg.Warn("dead letter exceeded max retries, dropping",
				loggateway.Str("id", id),
				loggateway.Str("source", msgCopy.Source),
				loggateway.Int("max_retries", msgCopy.MaxRetries))
		}
	}
	return err
}

// Discard 丢弃指定 ID 的消息。
func (q *DeadLetterQueue) Discard(id string) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range q.messages {
		if q.messages[i].ID == id {
			q.messages = append(q.messages[:i], q.messages[i+1:]...)
			return true
		}
	}
	return false
}

// Len 返回队列长度。
func (q *DeadLetterQueue) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.messages)
}

// Purge 清空队列。
func (q *DeadLetterQueue) Purge() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = nil
}
