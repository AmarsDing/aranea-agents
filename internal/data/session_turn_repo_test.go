package data

import (
	"errors"
	"testing"

	"aranea-agents/internal/data/ent"
)

func TestIsSessionTurnUniqueConflict(t *testing.T) {
	d := DialectPostgres
	if !isSessionTurnUniqueConflict(d, &ent.ConstraintError{}) {
		t.Fatal("ent ConstraintError should retry")
	}
	if isSessionTurnUniqueConflict(d, errors.New("connection reset")) {
		t.Fatal("generic error should not retry")
	}
	if isSessionTurnUniqueConflict(d, nil) {
		t.Fatal("nil should not retry")
	}
}

func TestCreateSessionTurnMaxAttempts(t *testing.T) {
	if createSessionTurnMaxAttempts < 2 {
		t.Fatalf("need bounded retries >= 2, got %d", createSessionTurnMaxAttempts)
	}
}
