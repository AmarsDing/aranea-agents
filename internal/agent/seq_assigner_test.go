package agent

import (
	"sync"
	"testing"
)

func TestSeqAssigner_NextSeq_Monotonic(t *testing.T) {
	sa := NewSeqAssigner()
	expected := []int64{1, 2, 3, 4, 5}
	for i, want := range expected {
		got := sa.NextSeq("session-1")
		if got != want {
			t.Fatalf("NextSeq[%d]: expected %d, got %d", i, want, got)
		}
	}
}

func TestSeqAssigner_NextSeq_PerSessionIsolation(t *testing.T) {
	sa := NewSeqAssigner()
	// session-1 拿 3 个
	sa.NextSeq("session-1")
	sa.NextSeq("session-1")
	sa.NextSeq("session-1")
	// session-2 应该从 1 开始
	got := sa.NextSeq("session-2")
	if got != 1 {
		t.Fatalf("session-2 first seq: expected 1, got %d", got)
	}
	// session-1 继续应该 4
	got = sa.NextSeq("session-1")
	if got != 4 {
		t.Fatalf("session-1 after session-2: expected 4, got %d", got)
	}
}

func TestSeqAssigner_NextSeq_Concurrent(t *testing.T) {
	sa := NewSeqAssigner()
	const goroutines = 50
	const perG = 100
	var wg sync.WaitGroup
	results := make(chan int64, goroutines*perG)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				results <- sa.NextSeq("session-concurrent")
			}
		}()
	}
	wg.Wait()
	close(results)
	seen := make(map[int64]bool, goroutines*perG)
	for seq := range results {
		if seq < 1 || seq > int64(goroutines*perG) {
			t.Fatalf("seq out of range: %d", seq)
		}
		if seen[seq] {
			t.Fatalf("duplicate seq: %d", seq)
		}
		seen[seq] = true
	}
	if len(seen) != goroutines*perG {
		t.Fatalf("expected %d unique seqs, got %d", goroutines*perG, len(seen))
	}
}

func TestSeqAssigner_RestoreFromDB(t *testing.T) {
	sa := NewSeqAssigner()
	sa.RestoreFromDB("session-restore", 100)
	got := sa.NextSeq("session-restore")
	if got != 101 {
		t.Fatalf("after restore: expected 101, got %d", got)
	}
}

func TestSeqAssigner_RestoreFromDB_DoesNotLower(t *testing.T) {
	sa := NewSeqAssigner()
	sa.NextSeq("session-x")          // seq=1
	sa.NextSeq("session-x")          // seq=2
	sa.RestoreFromDB("session-x", 1) // should not lower to 1
	got := sa.NextSeq("session-x")
	if got != 3 {
		t.Fatalf("restore should not lower seq: expected 3, got %d", got)
	}
}
