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
		reason string
	}{
		{apierror.NotFound("AGENT", "not found"), http.StatusNotFound, "AGENT_NOT_FOUND"},
		{apierror.BadRequest("CHAT", "bad"), http.StatusBadRequest, "CHAT_BAD_REQUEST"},
		{apierror.Unauthorized("AUTH", "unauth"), http.StatusUnauthorized, "AUTH_UNAUTHORIZED"},
		{apierror.Forbidden("ADMIN", "forbid"), http.StatusForbidden, "ADMIN_FORBIDDEN"},
		{apierror.Conflict("SESSION", "conflict"), http.StatusConflict, "SESSION_CONFLICT"},
		{apierror.Unavailable("DATA", "unavailable"), http.StatusServiceUnavailable, "DATA_UNAVAILABLE"},
		{apierror.RateLimit("LLM", "rate limited"), http.StatusTooManyRequests, "LLM_RATE_LIMITED"},
		{apierror.BadRequest("CHAT_QUEUE_FULL", "pending queue is full for this session"), http.StatusBadRequest, "CHAT_QUEUE_FULL"},
		{apierror.Internal("BIZ", "internal details"), http.StatusInternalServerError, "BIZ_INTERNAL"},
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
		if kerr.Reason != c.reason {
			t.Errorf("code %s: expected reason %q, got %q", c.apiErr.Code, c.reason, kerr.Reason)
		}
	}
}

func TestToKratosInternalSanitizes(t *testing.T) {
	ae := apierror.Internal("AGENT", "sensitive db error: connection refused")
	ke := apierror.ToKratos(ae)
	var kerr *kerrors.Error
	if !errors.As(ke, &kerr) {
		t.Fatal("ToKratos did not return *kerrors.Error")
	}
	if kerr.Message != "internal error" {
		t.Errorf("CodeInternal should sanitize message, got %q", kerr.Message)
	}
	if kerr.Reason != "AGENT_INTERNAL" {
		t.Errorf("expected reason AGENT_INTERNAL, got %q", kerr.Reason)
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

func TestToKratosFallbackSanitizes(t *testing.T) {
	// Unknown error type should become 500 with sanitized message.
	ke := apierror.ToKratos(errors.New("some internal detail"))
	var kerr *kerrors.Error
	if !errors.As(ke, &kerr) {
		t.Fatal("ToKratos did not return *kerrors.Error for unknown error")
	}
	if kerr.Message != "internal error" {
		t.Errorf("fallback should sanitize message, got %q", kerr.Message)
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

func TestIsComparesCodeAndDomain(t *testing.T) {
	agentNotFound := apierror.NotFound("AGENT", "agent not found")
	sessionNotFound := apierror.NotFound("SESSION", "session not found")
	agentBadRequest := apierror.BadRequest("AGENT", "bad input")
	agentConflict := apierror.Conflict("AGENT", "key conflict")
	dataConflict := apierror.Conflict("DATA", "duplicate")

	// Same Code, different Domain → Is returns false
	if errors.Is(agentNotFound, sessionNotFound) {
		t.Error("Is should NOT match errors with same Code but different Domain")
	}

	// Same Code AND Domain → Is returns true
	agentNotFound2 := apierror.NotFound("AGENT", "another agent not found")
	if !errors.Is(agentNotFound, agentNotFound2) {
		t.Error("Is should match errors with same Code and Domain")
	}

	// Different Code → Is returns false
	if errors.Is(agentNotFound, agentBadRequest) {
		t.Error("Is should not match errors with different Code")
	}

	// Same Code (Conflict), different Domain → Is returns false
	if errors.Is(agentConflict, dataConflict) {
		t.Error("Is should NOT match errors with same Code but different Domain (Conflict)")
	}

	// Not an apierror → Is returns false
	if errors.Is(agentNotFound, errors.New("plain")) {
		t.Error("Is should not match non-apierror targets")
	}
}
