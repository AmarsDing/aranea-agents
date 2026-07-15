package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// denyAllAuthorizer denies all ownership checks.
type denyAllAuthorizer struct{}

func (denyAllAuthorizer) CheckOwnership(_ context.Context, _, _ string) error {
	return errors.New("ownership denied")
}

// allowAllAuthorizer allows all ownership checks.
type allowAllAuthorizer struct{}

func (allowAllAuthorizer) CheckOwnership(_ context.Context, _, _ string) error {
	return nil
}

const testSecret = "test-secret-at-least-32-characters-long"

// TestHandleWS_NonGlobalMode_DeniesWhenOwnershipFails verifies that a
// non-admin user subscribing to a specific session_id is rejected with 403
// when the SessionAuthorizer denies ownership.
func TestHandleWS_NonGlobalMode_DeniesWhenOwnershipFails(t *testing.T) {
	os.Unsetenv("KRATOS_HTTP_AUTH_DISABLED")
	t.Setenv("DEPLOY_ENV", "production")
	prev := auth.SetSecretForTesting(testSecret)
	defer auth.SetSecretForTesting(prev)

	// Generate a non-admin token
	token, err := auth.GenerateToken(42, "user", testSecret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil, nil, nil, nil, loggateway.NewNoop(), nil,
		denyAllAuthorizer{},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/ws?session_id=sess-1", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	srv.handleWS(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for denied ownership, got %d body=%q", rec.Code, rec.Body.String())
	}
}

// TestHandleWS_NonGlobalMode_AllowsWhenOwnershipPasses verifies that a
// non-admin user subscribing to a specific session_id is allowed (proceeds
// past the ownership check) when the SessionAuthorizer allows it.
func TestHandleWS_NonGlobalMode_AllowsWhenOwnershipPasses(t *testing.T) {
	os.Unsetenv("KRATOS_HTTP_AUTH_DISABLED")
	t.Setenv("DEPLOY_ENV", "production")
	prev := auth.SetSecretForTesting(testSecret)
	defer auth.SetSecretForTesting(prev)

	token, err := auth.GenerateToken(42, "user", testSecret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil, nil, nil, nil, loggateway.NewNoop(), nil,
		allowAllAuthorizer{},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/ws?session_id=sess-1", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	srv.handleWS(rec, req)

	// The handler should proceed past auth + ownership check.
	// It will likely fail at WS upgrade (httptest can't upgrade), but
	// it must NOT return 401 or 403.
	if rec.Code == http.StatusForbidden {
		t.Fatalf("expected not 403 with allowed ownership, got %d body=%q", rec.Code, rec.Body.String())
	}
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("expected not 401 with valid token, got %d", rec.Code)
	}
}

// TestHandleWS_AdminBypassesOwnershipCheck verifies that an admin user
// bypasses the session ownership check entirely.
func TestHandleWS_AdminBypassesOwnershipCheck(t *testing.T) {
	os.Unsetenv("KRATOS_HTTP_AUTH_DISABLED")
	t.Setenv("DEPLOY_ENV", "production")
	prev := auth.SetSecretForTesting(testSecret)
	defer auth.SetSecretForTesting(prev)

	token, err := auth.GenerateToken(1, "admin", testSecret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	srv := NewWSServerFromInfra(
		&conf.Server{Ws: &conf.Server_WS{Enable: true}},
		&event.Infra{MonitorEventBus: event.NewMonitorBus(loggateway.NewNoop())},
		nil, nil, nil, nil, loggateway.NewNoop(), nil,
		denyAllAuthorizer{},
	)

	req := httptest.NewRequest(http.MethodGet, "/v1/ws?session_id=sess-1", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()

	srv.handleWS(rec, req)

	// Admin should bypass the ownership check (denyAllAuthorizer would
	// reject non-admins). Must NOT return 401 or 403.
	if rec.Code == http.StatusForbidden {
		t.Fatalf("admin should bypass ownership check, got 403 body=%q", rec.Body.String())
	}
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("admin token should be valid, got 401 body=%q", rec.Body.String())
	}
}
