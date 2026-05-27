package client_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/client"
)

func makeTestClient(srv *httptest.Server) *client.Client {
	return client.NewClient(srv.URL, "test-token", "dev", false, nil)
}

func assertCLIError(t *testing.T, err error, wantCode string, wantStatus int) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ce, ok := err.(*cli.CLIError)
	if !ok {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != wantCode {
		t.Errorf("code: got %q, want %q", ce.Code, wantCode)
	}
	if ce.HTTPStatus != wantStatus {
		t.Errorf("http_status: got %d, want %d", ce.HTTPStatus, wantStatus)
	}
}

func TestErrorDecode_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"reason":"VALIDATION_ERROR","message":"bad request"}`))
	}))
	defer srv.Close()
	c := makeTestClient(srv)
	err := c.Do(nil, http.MethodPost, "/v1/test", nil, nil) //nolint:staticcheck
	assertCLIError(t, err, "VALIDATION_ERROR", 400)
	if code := cli.ExitCodeOf(err); code != cli.ExitBackendBizError {
		t.Errorf("exit code: got %d, want %d", code, cli.ExitBackendBizError)
	}
}

func TestErrorDecode_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"reason":"UNAUTHENTICATED","message":"Token is invalid"}`))
	}))
	defer srv.Close()
	c := makeTestClient(srv)
	err := c.Do(nil, http.MethodGet, "/v1/test", nil, nil) //nolint:staticcheck
	assertCLIError(t, err, "UNAUTHENTICATED", 401)
	if code := cli.ExitCodeOf(err); code != cli.ExitAuthError {
		t.Errorf("exit code: got %d, want %d", code, cli.ExitAuthError)
	}
}

func TestErrorDecode_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"reason":"FORBIDDEN","message":"Access denied"}`))
	}))
	defer srv.Close()
	c := makeTestClient(srv)
	err := c.Do(nil, http.MethodGet, "/v1/test", nil, nil) //nolint:staticcheck
	assertCLIError(t, err, "FORBIDDEN", 403)
	if code := cli.ExitCodeOf(err); code != cli.ExitAuthError {
		t.Errorf("exit code: got %d, want %d", code, cli.ExitAuthError)
	}
}

func TestErrorDecode_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"reason":"NOT_FOUND","message":"not found"}`))
	}))
	defer srv.Close()
	c := makeTestClient(srv)
	err := c.Do(nil, http.MethodGet, "/v1/test", nil, nil) //nolint:staticcheck
	assertCLIError(t, err, "NOT_FOUND", 404)
	if code := cli.ExitCodeOf(err); code != cli.ExitBackendBizError {
		t.Errorf("exit code: got %d, want %d", code, cli.ExitBackendBizError)
	}
}

func TestErrorDecode_409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte(`{"reason":"SKILL_IMPORT_BLOCKED","message":"conflict"}`))
	}))
	defer srv.Close()
	c := makeTestClient(srv)
	err := c.Do(nil, http.MethodPost, "/v1/test", nil, nil) //nolint:staticcheck
	assertCLIError(t, err, "SKILL_IMPORT_BLOCKED", 409)
	if code := cli.ExitCodeOf(err); code != cli.ExitBackendBizError {
		t.Errorf("exit code: got %d, want %d", code, cli.ExitBackendBizError)
	}
}

func TestErrorDecode_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"message":"internal error"}`))
	}))
	defer srv.Close()
	c := makeTestClient(srv)
	err := c.Do(nil, http.MethodGet, "/v1/test", nil, nil) //nolint:staticcheck
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
	if code := cli.ExitCodeOf(err); code != cli.ExitNetworkError {
		t.Errorf("exit code: got %d, want %d", code, cli.ExitNetworkError)
	}
}

func TestExitCodeOf_UserCanceled(t *testing.T) {
	err := &cli.CLIError{Code: "USER_CANCELED"}
	if code := cli.ExitCodeOf(err); code != cli.ExitUserCanceled {
		t.Errorf("got %d, want %d", code, cli.ExitUserCanceled)
	}
}

func TestExitCodeOf_SkillBlocked(t *testing.T) {
	err := &cli.CLIError{Code: "SKILL_IMPORT_BLOCKED"}
	if code := cli.ExitCodeOf(err); code != cli.ExitConflictBlocked {
		t.Errorf("got %d, want %d", code, cli.ExitConflictBlocked)
	}
}
