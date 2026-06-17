package data

import (
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"

	"github.com/lib/pq"
)

// TestEntErrToBizErr_PostgresUniqueViolation verifies that a Postgres unique
// violation (SQLSTATE 23505) is translated to CodeConflict.
func TestEntErrToBizErr_PostgresUniqueViolation(t *testing.T) {
	pgErr := &pq.Error{Code: "23505", Message: "duplicate key value violates unique constraint"}
	got := entErrToBizErr(pgErr, "TEST")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeConflict {
		t.Fatalf("expected code CONFLICT for unique violation, got %s", ae.Code)
	}
	if !errors.Is(got, pgErr) {
		t.Fatal("expected original pq.Error in chain")
	}
}

// TestEntErrToBizErr_PostgresForeignKeyViolation verifies that a Postgres FK
// violation (SQLSTATE 23503) is translated to CodeConflict.
func TestEntErrToBizErr_PostgresForeignKeyViolation(t *testing.T) {
	pgErr := &pq.Error{Code: "23503", Message: "insert or update on table violates foreign key constraint"}
	got := entErrToBizErr(pgErr, "TEST")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeConflict {
		t.Fatalf("expected code CONFLICT for FK violation, got %s", ae.Code)
	}
}

// TestEntErrToBizErr_PostgresNotNullViolation verifies that a Postgres NOT NULL
// violation (SQLSTATE 23502) is translated to CodeBadRequest.
func TestEntErrToBizErr_PostgresNotNullViolation(t *testing.T) {
	pgErr := &pq.Error{Code: "23502", Message: "null value in column violates not-null constraint"}
	got := entErrToBizErr(pgErr, "TEST")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code BAD_REQUEST for not-null violation, got %s", ae.Code)
	}
}

// TestEntErrToBizErr_PostgresCheckViolation verifies that a Postgres CHECK
// violation (SQLSTATE 23514) is translated to CodeBadRequest.
func TestEntErrToBizErr_PostgresCheckViolation(t *testing.T) {
	pgErr := &pq.Error{Code: "23514", Message: "new row for relation violates check constraint"}
	got := entErrToBizErr(pgErr, "TEST")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code BAD_REQUEST for check violation, got %s", ae.Code)
	}
}

// TestEntErrToBizErr_PostgresUndefinedTable verifies that a Postgres undefined
// table error (SQLSTATE 42P01) is translated to CodeInternal (not a user error).
func TestEntErrToBizErr_PostgresUndefinedTable(t *testing.T) {
	pgErr := &pq.Error{Code: "42P01", Message: "relation does not exist"}
	got := entErrToBizErr(pgErr, "TEST")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeInternal {
		t.Fatalf("expected code INTERNAL for undefined table, got %s", ae.Code)
	}
}
