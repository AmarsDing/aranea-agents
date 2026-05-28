package config

import (
	"encoding/json"
	"testing"
)

func TestParseServerConfigJSON_Empty(t *testing.T) {
	c, err := ParseServerConfigJSON("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Transport != "" {
		t.Fatalf("transport=%q", c.Transport)
	}
}

func TestNormalizeTransport_StreamableHTTP(t *testing.T) {
	if got := NormalizeTransport("streamable_http"); got != "streamable" {
		t.Fatalf("got %q", got)
	}
}

func TestParseTransport_Aliases(t *testing.T) {
	cases := []struct {
		input string
		want  Transport
	}{
		{"stdio", TransportStdio},
		{"SSE", TransportSSE},
		{"streamable_http", TransportStreamable},
		{"streamablehttp", TransportStreamable},
		{"http", TransportStreamable},
		{"streamable", TransportStreamable},
	}
	for _, tc := range cases {
		got, err := ParseTransport(tc.input)
		if err != nil {
			t.Errorf("ParseTransport(%q): %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseTransport(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseTransport_Unknown(t *testing.T) {
	_, err := ParseTransport("websocket")
	if err == nil {
		t.Error("expected error for unknown transport")
	}
}

func TestTransportUnmarshalJSON_AutoNormalize(t *testing.T) {
	raw := `{"transport": "streamable_http"}`
	var c ServerConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatal(err)
	}
	if c.Transport != TransportStreamable {
		t.Fatalf("transport=%q, want %q", c.Transport, TransportStreamable)
	}
}
