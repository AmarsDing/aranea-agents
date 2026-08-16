package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"aranea-agents/internal/conf"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

func TestHandleWS_AuthBypassWithoutToken(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "dev")

	srv := NewWSServerFromInfra(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, &event.Infra{
		MonitorEventBus: event.NewMonitorBus(nil),
	}, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil)
	srv.SetAllowNilSessionAuth(true)
	if srv == nil {
		t.Fatal("expected WSServer")
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/ws?session_id=*", nil)
	req.Header.Set("Origin", "http://localhost:9001")
	rec := httptest.NewRecorder()

	// Without a valid Upgrade client this returns after auth; 401 must not occur.
	srv.handleWS(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("expected not 401 with auth bypass, got %d body=%q", rec.Code, rec.Body.String())
	}
	// httptest cannot complete WS upgrade; non-401 is enough (often 500 from upgrader).
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("unauthorized")
	}
	_ = strings.Contains(rec.Body.String(), "unauthorized")
}

func TestHandleWS_RequiresTokenWhenAuthOn(t *testing.T) {
	os.Unsetenv("KRATOS_HTTP_AUTH_DISABLED")
	t.Setenv("DEPLOY_ENV", "production")
	t.Setenv("KRATOS_AUTH_SECRET", "test-secret-at-least-32-characters-long")

	srv := NewWSServerFromInfra(&conf.Server{Ws: &conf.Server_WS{Enable: true}}, &event.Infra{
		MonitorEventBus: event.NewMonitorBus(nil),
	}, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil)
	srv.SetAllowNilSessionAuth(true)
	req := httptest.NewRequest(http.MethodGet, "/v1/ws?session_id=s1", nil)
	rec := httptest.NewRecorder()
	srv.handleWS(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}
}
