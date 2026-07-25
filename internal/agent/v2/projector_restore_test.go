package v2

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestProjectorFactory_RestoreSeqIfNeeded(t *testing.T) {
	t.Parallel()
	sa := NewDefaultSeqAssigner()
	f := NewProjectorFactory(nil, sa, nil, loggateway.NewNoop())

	f.RestoreSeqIfNeeded("sess-a", 5)
	if got := sa.NextSeq("sess-a"); got != 6 {
		t.Fatalf("first restore NextSeq = %d, want 6", got)
	}
	// Second restore is skipped by per-process seqRestored gate.
	f.RestoreSeqIfNeeded("sess-a", 100)
	if got := sa.NextSeq("sess-a"); got != 7 {
		t.Fatalf("after skipped restore NextSeq = %d, want 7 (not 101)", got)
	}
}
