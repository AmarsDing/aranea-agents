package v2

import "testing"

func TestDefaultSeqAssigner_RestoreAtLeast(t *testing.T) {
	t.Parallel()
	sa := NewDefaultSeqAssigner()
	sa.RestoreAtLeast("sess-1", 10)
	got := sa.NextSeq("sess-1")
	if got != 11 {
		t.Fatalf("NextSeq after RestoreAtLeast(10) = %d, want 11", got)
	}
	// Lower restore must not rewind.
	sa.RestoreAtLeast("sess-1", 5)
	got = sa.NextSeq("sess-1")
	if got != 12 {
		t.Fatalf("NextSeq after lower restore = %d, want 12", got)
	}
}
