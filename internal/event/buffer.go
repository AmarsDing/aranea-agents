package event

import (
	"context"
	"sync"
	"time"

	"aranea-agents/pkg/safego"
)

type Buffer struct {
	mu      sync.RWMutex
	buffers map[string]*ringBuffer
	cap     int
	ttl     time.Duration
	lastAcc map[string]time.Time
	stopCh  chan struct{}
}

type ringBuffer struct {
	events []Envelope
	head   int
	size   int
}

func NewBuffer() *Buffer {
	cap := 200
	ttl := 30 * time.Minute
	b := &Buffer{
		buffers: make(map[string]*ringBuffer),
		cap:     cap,
		ttl:     ttl,
		lastAcc: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}
	safego.Go(context.Background(), "buffer-evict", b.evictLoop)
	return b
}

func (b *Buffer) evictLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.evictExpired()
		case <-b.stopCh:
			return
		}
	}
}

func (b *Buffer) evictExpired() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	for sid, last := range b.lastAcc {
		if now.Sub(last) > b.ttl {
			delete(b.buffers, sid)
			delete(b.lastAcc, sid)
		}
	}
}

func (b *Buffer) Close() {
	close(b.stopCh)
}

func (b *Buffer) Append(env Envelope) {
	if env.SessionID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastAcc[env.SessionID] = time.Now()
	buf, ok := b.buffers[env.SessionID]
	if !ok {
		buf = &ringBuffer{
			events: make([]Envelope, b.cap),
		}
		b.buffers[env.SessionID] = buf
	}
	buf.events[buf.head] = env
	buf.head = (buf.head + 1) % b.cap
	if buf.size < b.cap {
		buf.size++
	}
}

func (b *Buffer) Replay(sessionID, lastEventID string) []Envelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastAcc[sessionID] = time.Now()

	buf, ok := b.buffers[sessionID]
	if !ok || buf.size == 0 {
		return nil
	}

	if lastEventID == "" {
		result := make([]Envelope, buf.size)
		start := (buf.head - buf.size + b.cap) % b.cap
		for i := 0; i < buf.size; i++ {
			result[i] = buf.events[(start+i)%b.cap]
		}
		return result
	}

	start := (buf.head - buf.size + b.cap) % b.cap
	found := false
	foundIdx := 0
	for i := 0; i < buf.size; i++ {
		if buf.events[(start+i)%b.cap].ID == lastEventID {
			found = true
			foundIdx = i + 1
			break
		}
	}
	if !found {
		result := make([]Envelope, buf.size)
		for i := 0; i < buf.size; i++ {
			result[i] = buf.events[(start+i)%b.cap]
		}
		return result
	}

	count := buf.size - foundIdx
	if count <= 0 {
		return nil
	}
	result := make([]Envelope, count)
	for i := 0; i < count; i++ {
		result[i] = buf.events[(start+foundIdx+i)%b.cap]
	}
	return result
}

func (b *Buffer) RemoveSession(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.buffers, sessionID)
	delete(b.lastAcc, sessionID)
}
