package auth

import "testing"

func TestHealthAuthInfoBypass(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "1")
	t.Setenv("DEPLOY_ENV", "dev")
	info := HealthAuthInfo()
	if info.AuthMode != "bypass" {
		t.Fatalf("auth_mode=%q want bypass", info.AuthMode)
	}
	if info.WSPath != "/v1/ws" {
		t.Fatalf("ws_path=%q", info.WSPath)
	}
}
