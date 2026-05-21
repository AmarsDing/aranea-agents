package conf

import "testing"

func TestServer_ProcessLogEnabled(t *testing.T) {
	if (&Server{}).ProcessLogEnabled() != true {
		t.Fatal("nil monitor block should default to enabled")
	}
	if (&Server{Monitor: &Monitor{ProcessLogEnabled: false}}).ProcessLogEnabled() != false {
		t.Fatal("explicit false")
	}
	if (&Server{Monitor: &Monitor{ProcessLogEnabled: true}}).ProcessLogEnabled() != true {
		t.Fatal("explicit true")
	}
}
