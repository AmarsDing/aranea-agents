package config

import "testing"

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
