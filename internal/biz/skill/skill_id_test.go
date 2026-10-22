package skill

import (
	"encoding/hex"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNewSkillID_Format 钉死 ID 形态：skill_ + 32 hex（128bit）。
func TestNewSkillID_Format(t *testing.T) {
	id := NewSkillID()
	if !strings.HasPrefix(id, "skill_") {
		t.Fatalf("expected skill_ prefix, got=%q", id)
	}
	raw := strings.TrimPrefix(id, "skill_")
	if len(raw) != 32 {
		t.Fatalf("expected 32 hex chars, got=%d (%q)", len(raw), raw)
	}
	if _, err := hex.DecodeString(raw); err != nil {
		t.Fatalf("expected valid hex, got=%v", err)
	}
}

// TestNewSkillID_ConcurrentUniqueness 并发万级无碰撞（P0-7 A14③）：
// 10000 个 goroutine 并发生成，全部必须唯一——替代旧 skill_%d
// （UnixNano 时钟回拨/同纳秒可碰撞）的兜底语义可信。
func TestNewSkillID_ConcurrentUniqueness(t *testing.T) {
	const n = 10000
	ids := make([]string, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			ids[i] = NewSkillID()
		}(i)
	}
	wg.Wait()

	seen := make(map[string]struct{}, n)
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

// TestNewSkillID_TimestampSegment 钉死时间戳段毫秒精度（评审注 ④）：
// 高 48bit（前 12 hex）必须落在生成时刻的合理毫秒窗口内。
func TestNewSkillID_TimestampSegment(t *testing.T) {
	before := time.Now().UnixMilli()
	id := NewSkillID()
	after := time.Now().UnixMilli()

	raw := strings.TrimPrefix(id, "skill_")
	if len(raw) != 32 {
		t.Fatalf("expected 32 hex chars, got=%d", len(raw))
	}
	b, err := hex.DecodeString(raw[:12])
	if err != nil {
		t.Fatalf("decode timestamp segment: %v", err)
	}
	// 48-bit 大端时间戳。
	var ts int64
	for _, by := range b {
		ts = ts<<8 | int64(by)
	}
	if ts < before || ts > after+1 {
		t.Fatalf("timestamp segment %d out of [%d, %d]", ts, before, after+1)
	}
}
