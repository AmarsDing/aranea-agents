package content

import "testing"

func TestContentTransitions(t *testing.T) {
	if !CanTransition(StatusCandidate, StatusSelected) {
		t.Fatal("candidate→selected")
	}
	if CanTransition(StatusCandidate, StatusApproved) {
		t.Fatal("candidate→approved must be illegal")
	}
	if _, err := Transition(StatusInReview, StatusApproved); err != nil {
		t.Fatal(err)
	}
}

func TestCreateFromIngestDedupe(t *testing.T) {
	uc := NewUsecase(NewMemoryRepo())
	a, dup, err := uc.CreateFromIngest(Item{WorkspaceID: "w", SourceURL: "https://x.example/a", Title: "A", Body: "b"})
	if err != nil || dup {
		t.Fatalf("first: dup=%v err=%v", dup, err)
	}
	b, dup, err := uc.CreateFromIngest(Item{WorkspaceID: "w", SourceURL: "https://x.example/a", Title: "A2", Body: "b2"})
	if err != nil || !dup || a.ID != b.ID {
		t.Fatalf("second: dup=%v id=%s/%s err=%v", dup, a.ID, b.ID, err)
	}
}
