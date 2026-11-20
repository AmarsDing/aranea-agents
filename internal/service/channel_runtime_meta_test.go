package service

import "testing"

func TestStripRuntimeMetadata(t *testing.T) {
	got := stripRuntimeMetadata(`{"runtime_connected":true,"runtime_connected_since":"x","last_error_message":"boom"}`)
	if got != `{"last_error_message":"boom"}` {
		t.Fatalf("got %s", got)
	}
	if stripRuntimeMetadata("") != "" || stripRuntimeMetadata(`{"a":1}`) != `{"a":1}` {
		t.Fatal("untouched inputs must pass through")
	}
}
