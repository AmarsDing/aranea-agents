package shared

import (
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestErrUsageScopeRequired(t *testing.T) {
	if !strings.Contains(ErrUsageScopeRequired.Error(), "usage scope required") {
		t.Fatalf("expected message to contain 'usage scope required', got %q", ErrUsageScopeRequired.Error())
	}
}

func TestErrBudgetAlertNotFound(t *testing.T) {
	if !strings.Contains(ErrBudgetAlertNotFound.Error(), "budget alert not found") {
		t.Fatalf("expected message to contain 'budget alert not found', got %q", ErrBudgetAlertNotFound.Error())
	}
}

func TestErrQuotaNotFound(t *testing.T) {
	if !strings.Contains(ErrQuotaNotFound.Error(), "usage quota not configured") {
		t.Fatalf("expected message to contain 'usage quota not configured', got %q", ErrQuotaNotFound.Error())
	}
}

func TestErrMessageDuplicate(t *testing.T) {
	if !strings.Contains(ErrMessageDuplicate.Error(), "message duplicate constraint") {
		t.Fatalf("expected message to contain 'message duplicate constraint', got %q", ErrMessageDuplicate.Error())
	}
}

func TestErrAdminNotFound(t *testing.T) {
	e := kerrors.FromError(ErrAdminNotFound)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "ADMIN" {
		t.Fatalf("expected reason ADMIN, got %q", e.Reason)
	}
}

func TestErrNotFound(t *testing.T) {
	e := kerrors.FromError(ErrNotFound)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "NOT_FOUND" {
		t.Fatalf("expected reason NOT_FOUND, got %q", e.Reason)
	}
}

func TestErrGraphSaveRun(t *testing.T) {
	e := kerrors.FromError(ErrGraphSaveRun)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "GRAPH" {
		t.Fatalf("expected reason GRAPH, got %q", e.Reason)
	}
}

func TestErrGraphInvalidStatus(t *testing.T) {
	e := kerrors.FromError(ErrGraphInvalidStatus)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "GRAPH" {
		t.Fatalf("expected reason GRAPH, got %q", e.Reason)
	}
}

func TestErrGraphResume(t *testing.T) {
	e := kerrors.FromError(ErrGraphResume)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "GRAPH" {
		t.Fatalf("expected reason GRAPH, got %q", e.Reason)
	}
}

func TestErrGraphTemplateNotFound(t *testing.T) {
	e := kerrors.FromError(ErrGraphTemplateNotFound)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "GRAPH_TEMPLATE" {
		t.Fatalf("expected reason GRAPH_TEMPLATE, got %q", e.Reason)
	}
}

func TestErrQuotaUnsupportedScope(t *testing.T) {
	e := kerrors.FromError(ErrQuotaUnsupportedScope)
	if e == nil {
		t.Fatal("expected kratos error")
	}
	if e.Reason != "USAGE_QUOTA" {
		t.Fatalf("expected reason USAGE_QUOTA, got %q", e.Reason)
	}
}

func TestJSONStringList_WhitespaceOnly(t *testing.T) {
	got, err := JSONStringList("   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestJSONStringList_SingleElement(t *testing.T) {
	got, err := JSONStringList(`["a"]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("expected [\"a\"], got %v", got)
	}
}

func TestJSONStringList_NestedArray(t *testing.T) {
	_, err := JSONStringList(`[1,2]`)
	if err == nil {
		t.Fatal("expected error for non-string array elements")
	}
}

func TestPageToLimitOffset_BoundaryPage1(t *testing.T) {
	limit, offset, _, _ := PageToLimitOffset(1, 1)
	if limit != 1 {
		t.Fatalf("expected limit=1, got %d", limit)
	}
	if offset != 0 {
		t.Fatalf("expected offset=0, got %d", offset)
	}
}

func TestPageToLimitOffset_BoundaryMaxSize(t *testing.T) {
	limit, offset, _, _ := PageToLimitOffset(1, 100)
	if limit != 100 {
		t.Fatalf("expected limit=100, got %d", limit)
	}
	if offset != 0 {
		t.Fatalf("expected offset=0, got %d", offset)
	}
}

func TestPageToLimitOffset_BoundaryOverMaxSize(t *testing.T) {
	limit, offset, _, _ := PageToLimitOffset(1, 101)
	if limit != 100 {
		t.Fatalf("expected limit=100 (capped), got %d", limit)
	}
	if offset != 0 {
		t.Fatalf("expected offset=0, got %d", offset)
	}
}
