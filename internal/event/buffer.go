package event

import (
	"sync"
)

type Buffer struct {
	mu      sync.RWMutex
	buffers map[string]*ringBuffer
	cap     int
}

type ringBuffer struct {
	events []Envelope
	head   int
	size   int
}

func NewBuffer() *Buffer {
	cap := 200
	return &Buffer{
		buffers: make(map[string]*ringBuffer),
		cap:     cap,
	}
}

func (b *Buffer) Append(env Envelope) {
	if env.SessionID == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
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
	b.mu.RLock()
	defer b.mu.RUnlock()

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
}
