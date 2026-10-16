package service

import (
	"context"
	"sync"
	"testing"
)

type memDurableClaimer struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

func (m *memDurableClaimer) TryClaim(_ context.Context, sessionID, requestID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.keys == nil {
		m.keys = make(map[string]struct{})
	}
	k := sessionID + "\x00" + requestID
	if _, ok := m.keys[k]; ok {
		return false, nil
	}
	m.keys[k] = struct{}{}
	return true, nil
}

func (m *memDurableClaimer) Release(_ context.Context, sessionID, requestID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, sessionID+"\x00"+requestID)
	return nil
}

func TestTurnIdemRegistry_DurableDuplicateSurvivesMemoryClear(t *testing.T) {
	r := newTurnIdemRegistry()
	durable := &memDurableClaimer{}
	r.setDurable(durable, nil)

	if !r.claim("s1", "pending-user-1") {
		t.Fatal("first claim must succeed")
	}
	// Simulate process restart: fresh memory registry, same durable store.
	r2 := newTurnIdemRegistry()
	r2.setDurable(durable, nil)
	if r2.claim("s1", "pending-user-1") {
		t.Fatal("durable duplicate must be rejected after memory clear")
	}
	r2.release("s1", "pending-user-1")
	if !r2.claim("s1", "pending-user-1") {
		t.Fatal("claim after release must succeed")
	}
}
