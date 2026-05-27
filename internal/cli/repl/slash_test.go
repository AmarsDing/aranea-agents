package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestHandleSlashDryRunExplicitOnOff(t *testing.T) {
	r := New(Config{})
	var out bytes.Buffer
	handleSlash("/dry-run on", r, &out)
	if !r.dryRun {
		t.Fatal("dry-run should be enabled")
	}
	handleSlash("/dry-run off", r, &out)
	if r.dryRun {
		t.Fatal("dry-run should be disabled")
	}
}

func TestHandleSlashUnsupportedCommandsAreExplicit(t *testing.T) {
	r := New(Config{})
	for _, cmd := range []string{"/session list", "/yes", "/tools", "/expand", "/copy"} {
		var out bytes.Buffer
		res := handleSlash(cmd, r, &out)
		if !res.Handled {
			t.Fatalf("%s was not handled", cmd)
		}
		if !strings.Contains(out.String(), "暂不可用") {
			t.Fatalf("%s output = %q, want explicit unavailable message", cmd, out.String())
		}
	}
}
