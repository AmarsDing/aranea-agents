package apierror_test

import (
	"errors"
	"net/http"
	"testing"

	"aranea-agents/pkg/apierror"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestConstructors(t *testing.T) {
	cases := []struct {
		err  *apierror.Error
		code apierror.Code
	}{
		{apierror.NotFound("agent", "not found"), apierror.CodeNotFound},
		{apierror.BadRequest("chat", "bad"), apierror.CodeBadRequest},
		{apierror.Unauthorized("auth", "unauth"), apierror.CodeUnauthorized},
		{apierror.Forbidden("admin", "forbid"), apierror.CodeForbidden},
		{apierror.Conflict("session", "conflict"), apierror.CodeConflict},
		{apierror.Internal("biz", "internal"), apierror.CodeInternal},
		{apierror.Unavailable("data", "unavailable"), apierror.CodeUnavailable},
	}
	for _, c := range cases {
		if c.err.Code != c.code {
			t.Errorf("expected code %s, got %s", c.code, c.err.Code)
		}
	}
}

func TestWrapNil(t *testing.T) {
	if apierror.Wrap(nil, apierror.CodeInternal, "x") != nil {
		t.Error("Wrap(nil) should return nil")
	}
}

func TestWrapDoubleWrap(t *testing.T) {
	orig := apierror.NotFound("agent", "orig")
	wrapped := apierror.Wrap(orig, apierror.CodeInternal, "data")
	if wrapped.Code != apierror.CodeNotFound {
		t.Error("Wrap should not change code of already-wrapped *Error")
	}
}

func TestFrom(t *testing.T) {
	orig := apierror.BadRequest("biz", "bad")
	chained := errors.Join(errors.New("outer"), orig)

	found, ok := apierror.From(chained)
	if !ok {
		t.Fatal("From should find *Error in chain")
	}
	if found.Code != apierror.CodeBadRequest {
		t.Errorf("unexpected code: %s", found.Code)
	}

	_, ok = apierror.From(errors.New("plain"))
	if ok {
		t.Error("From should return false for plain errors")
	}
}

func TestToKratos(t *testing.T) {
	cases := []struct {
		apiErr *apierror.Error
		status int
	}{
		{apierror.NotFound("x", "x"), http.StatusNotFound},
		{apierror.BadRequest("x", "x"), http.StatusBadRequest},
		{apierror.Unauthorized("x", "x"), http.StatusUnauthorized},
		{apierror.Forbidden("x", "x"), http.StatusForbidden},
		{apierror.Conflict("x", "x"), http.StatusConflict},
		{apierror.Unavailable("x", "x"), http.StatusServiceUnavailable},
		{apierror.Internal("x", "x"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		ke := apierror.ToKratos(c.apiErr)
		var kerr *kerrors.Error
		if !errors.As(ke, &kerr) {
			t.Errorf("ToKratos did not return *kerrors.Error for code %s", c.apiErr.Code)
			continue
		}
		if int(kerr.Code) != c.status {
			t.Errorf("code %s: expected HTTP %d, got %d", c.apiErr.Code, c.status, kerr.Code)
		}
	}
}

func TestToKratosNil(t *testing.T) {
	if apierror.ToKratos(nil) != nil {
		t.Error("ToKratos(nil) should return nil")
	}
}

func TestToKratosPassthrough(t *testing.T) {
	ke := kerrors.NotFound("X", "already kratos")
	out := apierror.ToKratos(ke)
	if out != ke {
		t.Error("ToKratos should pass through existing kerrors.Error unchanged")
	}
}

func TestWithMeta(t *testing.T) {
	orig := apierror.NotFound("agent", "missing")
	with := orig.WithMeta("id", "abc").WithMeta("tenant", "t1")
	if with.Meta["id"] != "abc" || with.Meta["tenant"] != "t1" {
		t.Error("WithMeta should carry key-value pairs")
	}
	if orig.Meta != nil {
		t.Error("WithMeta should not mutate original")
	}
}
