package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mcpv1 "aranea-agents/api/kratos/mcp_server/v1"
	"aranea-agents/internal/cli/client"
)

func TestValidateMCPServer(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/mcp-servers/validate" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"status":"valid","message":"config ok"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &mcpv1.ValidateMCPServerRequest{Enabled: true, ConfigJson: `{"command":"npx"}`}
	resp, err := c.ValidateMCPServer(context.Background(), req)
	if err != nil {
		t.Fatalf("ValidateMCPServer: %v", err)
	}
	if !resp.Ok {
		t.Error("expected ok=true")
	}
	if gotBody["enabled"] != true {
		t.Errorf("expected enabled=true in body, got %v", gotBody["enabled"])
	}
	if gotBody["configJson"] != `{"command":"npx"}` {
		t.Errorf("expected configJson in body, got %v", gotBody["configJson"])
	}
}
