package provider

import (
	"testing"
	"time"
)

func TestFirstByteTimeoutFromConfigJSON(t *testing.T) {
	if d := FirstByteTimeoutFromConfigJSON(""); d != 0 {
		t.Fatalf("empty = %v, want 0", d)
	}
	if d := FirstByteTimeoutFromConfigJSON(`{}`); d != 0 {
		t.Fatalf("empty object = %v, want 0", d)
	}
	if d := FirstByteTimeoutFromConfigJSON(`{not json`); d != 0 {
		t.Fatalf("invalid json = %v, want 0", d)
	}
	got := FirstByteTimeoutFromConfigJSON(`{"first_byte_timeout_sec":90,"context_window_k":256}`)
	if got != 90*time.Second {
		t.Fatalf("got %v, want 90s", got)
	}
}
