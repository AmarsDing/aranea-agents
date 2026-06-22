package tool

import "testing"

func TestValidateToolConfigFields_schemaMatch(t *testing.T) {
	err := validateToolConfigFields(ToolUpsertInput{
		Key:              "custom_api",
		DisplayName:      "Custom",
		ConfigSchemaJSON: `{"type":"object","properties":{"api_key":{"type":"string"}},"required":["api_key"]}`,
		ConfigJSON:       `{"api_key":"secret"}`,
	})
	if err != nil {
		t.Fatalf("expected valid config: %v", err)
	}
}

func TestValidateToolConfigFields_schemaReject(t *testing.T) {
	err := validateToolConfigFields(ToolUpsertInput{
		Key:              "custom_api",
		DisplayName:      "Custom",
		ConfigSchemaJSON: `{"type":"object","properties":{"api_key":{"type":"string"}},"required":["api_key"]}`,
		ConfigJSON:       `{}`,
	})
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}

func TestValidateMCPServerConfigJSON_stdio(t *testing.T) {
	if err := validateMCPServerConfigJSON(`{"transport":"stdio","command":"npx"}`); err != nil {
		t.Fatal(err)
	}
	if err := validateMCPServerConfigJSON(`{"transport":"stdio"}`); err == nil {
		t.Fatal("expected missing command error")
	}
}

func TestValidateMCPServerConfigJSON_sse(t *testing.T) {
	err := validateMCPServerConfigJSON(`{"transport":"sse","url":"https://mcp.example/sse"}`)
	if err != nil {
		if isNetworkError(err) {
			t.Skipf("skipping DNS-dependent test: %v", err)
		}
		t.Fatal(err)
	}
}

func TestValidateMCPServerConfigJSON_ssrf(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr bool
		network bool // true if this case requires DNS resolution and should be skipped offline
	}{
		{
			name:    "localhost blocked",
			json:    `{"transport":"sse","url":"http://localhost:8080/sse"}`,
			wantErr: true,
		},
		{
			name:    "private IP blocked",
			json:    `{"transport":"streamable_http","url":"http://192.168.1.1/mcp"}`,
			wantErr: true,
		},
		{
			name:    "loopback IP blocked",
			json:    `{"transport":"sse","url":"http://127.0.0.1:3000/sse"}`,
			wantErr: true,
		},
		{
			name:    "cloud metadata blocked",
			json:    `{"transport":"sse","url":"http://169.254.169.254/latest/meta-data/"}`,
			wantErr: true,
		},
		{
			name:    "ftp scheme blocked",
			json:    `{"transport":"sse","url":"ftp://evil.com/payload"}`,
			wantErr: true,
		},
		{
			name:    "public URL allowed",
			json:    `{"transport":"streamable_http","url":"https://mcp.example.com/api"}`,
			wantErr: false,
			network: true,
		},
		{
			name:    "stdio not affected by SSRF check",
			json:    `{"transport":"stdio","command":"npx"}`,
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMCPServerConfigJSON(tc.json)
			if tc.wantErr && err == nil {
				t.Error("expected SSRF error, got nil")
			}
			if !tc.wantErr && err != nil {
				if tc.network && isNetworkError(err) {
					t.Skipf("skipping DNS-dependent test: %v", err)
				}
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// isNetworkError returns true for errors that indicate no external network access.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsStr(msg, "no such host") ||
		containsStr(msg, "dial tcp") ||
		containsStr(msg, "lookup") ||
		containsStr(msg, "network") ||
		containsStr(msg, "connection refused")
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
