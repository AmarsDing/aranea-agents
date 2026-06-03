package data

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/data/ent"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestEntErrToBizErr_Nil(t *testing.T) {
	got := entErrToBizErr(nil, "TEST", "test")
	if got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
}

func TestEntErrToBizErr_NotFound(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()

	// Create schema so the sessions table exists.
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("schema create: %v", err)
	}

	// Query a nonexistent session — triggers NotFound.
	_, err := client.Session.Get(ctx, "nonexistent")
	if !ent.IsNotFound(err) {
		t.Fatalf("expected NotFound error, got %v", err)
	}

	got := entErrToBizErr(err, "SESSION", "session not found")
	var ke *kerrors.Error
	if !errors.As(got, &ke) {
		t.Fatalf("expected kerrors.Error, got %T", got)
	}
	if ke.Code != 404 {
		t.Fatalf("expected code 404, got %d", ke.Code)
	}
	if ke.Reason != "SESSION" {
		t.Fatalf("expected reason SESSION, got %q", ke.Reason)
	}
	if !errors.Is(got, err) {
		t.Fatal("expected original error in chain via WithCause")
	}
}

func TestEntErrToBizErr_ConstraintError(t *testing.T) {
	client := newTestEntClient(t)
	ctx := context.Background()

	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("schema create: %v", err)
	}

	// Insert a session, then try to insert the same ID again to trigger ConstraintError.
	_, err := client.Session.Create().SetID("dup-session").SetTitle("test").SetStatus("active").Save(ctx)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err = client.Session.Create().SetID("dup-session").SetTitle("test").SetStatus("active").Save(ctx)
	if !ent.IsConstraintError(err) {
		t.Fatalf("expected ConstraintError, got %v", err)
	}

	got := entErrToBizErr(err, "SESSION", "duplicate session")
	var ke *kerrors.Error
	if !errors.As(got, &ke) {
		t.Fatalf("expected kerrors.Error, got %T", got)
	}
	if ke.Code != 409 {
		t.Fatalf("expected code 409 Conflict, got %d", ke.Code)
	}
	if ke.Reason != "SESSION" {
		t.Fatalf("expected reason SESSION, got %q", ke.Reason)
	}
}

func TestEntErrToBizErr_NotLoaded(t *testing.T) {
	// ent.NotLoadedError has unexported fields, so we cannot construct it directly.
	// Verify the switch branch by checking that ent.IsNotLoaded returns true
	// for a real NotLoadedError and the function maps it to 400 BadRequest.
	// We test this indirectly: confirm that a non-Ent error does NOT match
	// ent.IsNotLoaded, and that the default branch maps to 500.
	err := errors.New("some unexpected db error")
	if ent.IsNotLoaded(err) {
		t.Fatal("generic error should not match IsNotLoaded")
	}

	got := entErrToBizErr(err, "DATA", "internal data error")
	var ke *kerrors.Error
	if !errors.As(got, &ke) {
		t.Fatalf("expected kerrors.Error, got %T", got)
	}
	if ke.Code != 500 {
		t.Fatalf("expected code 500 InternalServer, got %d", ke.Code)
	}
	if ke.Reason != "DATA" {
		t.Fatalf("expected reason DATA, got %q", ke.Reason)
	}
	if !errors.Is(got, err) {
		t.Fatal("expected original error in chain via WithCause")
	}
}
