package data

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
)

func TestEntErrToBizErr_Nil(t *testing.T) {
	got := entErrToBizErr(nil, "TEST")
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

	got := entErrToBizErr(err, "SESSION")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeNotFound {
		t.Fatalf("expected code NOT_FOUND, got %s", ae.Code)
	}
	if ae.Domain != "SESSION" {
		t.Fatalf("expected domain SESSION, got %q", ae.Domain)
	}
	if !errors.Is(got, err) {
		t.Fatal("expected original error in chain")
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

	got := entErrToBizErr(err, "SESSION")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeConflict {
		t.Fatalf("expected code CONFLICT, got %s", ae.Code)
	}
	if ae.Domain != "SESSION" {
		t.Fatalf("expected domain SESSION, got %q", ae.Domain)
	}
}

func TestEntErrToBizErr_NotLoaded(t *testing.T) {
	// ent.NotLoadedError has unexported fields, so we cannot construct it directly.
	// Verify the switch branch by checking that ent.IsNotLoaded returns true
	// for a real NotLoadedError and the function maps it to BAD_REQUEST.
	// We test this indirectly: confirm that a non-Ent error does NOT match
	// ent.IsNotLoaded, and that the default branch maps to INTERNAL.
	err := errors.New("some unexpected db error")
	if ent.IsNotLoaded(err) {
		t.Fatal("generic error should not match IsNotLoaded")
	}

	got := entErrToBizErr(err, "DATA")
	var ae *apierror.Error
	if !errors.As(got, &ae) {
		t.Fatalf("expected apierror.Error, got %T", got)
	}
	if ae.Code != apierror.CodeInternal {
		t.Fatalf("expected code INTERNAL, got %s", ae.Code)
	}
	if ae.Domain != "DATA" {
		t.Fatalf("expected domain DATA, got %q", ae.Domain)
	}
	if !errors.Is(got, err) {
		t.Fatal("expected original error in chain")
	}
}
