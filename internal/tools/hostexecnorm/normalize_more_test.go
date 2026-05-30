package hostexecnorm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeExecArgs_InvalidJSON(t *testing.T) {
	in := []byte(`not json at all`)
	out := NormalizeExecArgs(in)
	if string(out) != string(in) {
		t.Fatalf("expected passthrough for invalid JSON, got %s", out)
	}
}

func TestNormalizeExecArgs_EmptyJSON(t *testing.T) {
	in := []byte(`{}`)
	out := NormalizeExecArgs(in)
	if string(out) != string(in) {
		t.Fatalf("expected passthrough for empty JSON, got %s", out)
	}
}

func TestNormalizeExecArgs_NoWorkingDirOrWorkdir(t *testing.T) {
	in := []byte(`{"command":"ls"}`)
	out := NormalizeExecArgs(in)
	if string(out) != string(in) {
		t.Fatalf("expected passthrough when no working_dir or workdir, got %s", out)
	}
}

func TestNormalizeExecArgs_BothWorkingDirAndWorkdir(t *testing.T) {
	in := []byte(`{"command":"pwd","working_dir":"old","workdir":"existing"}`)
	out := NormalizeExecArgs(in)
	if string(out) != string(in) {
		t.Fatalf("expected passthrough when workdir already present, got %s", out)
	}
}

func TestNormalizeExecArgs_NestedFieldsPreserved(t *testing.T) {
	in := []byte(`{"command":"ls","working_dir":"sub","env":{"KEY":"VAL"},"timeout":30}`)
	out := NormalizeExecArgs(in)

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if m["workdir"] != "sub" {
		t.Fatalf("expected workdir=sub, got %v", m["workdir"])
	}
	if _, ok := m["working_dir"]; ok {
		t.Fatal("expected working_dir to be removed")
	}
	if m["command"] != "ls" {
		t.Fatalf("expected command=ls, got %v", m["command"])
	}
	env, ok := m["env"].(map[string]any)
	if !ok || env["KEY"] != "VAL" {
		t.Fatalf("expected env preserved, got %v", m["env"])
	}
}

func TestNormalizeExecArgs_WorkingDirMapsToWorkdir(t *testing.T) {
	in := []byte(`{"command":"pwd","working_dir":"/tmp"}`)
	out := NormalizeExecArgs(in)

	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if m["workdir"] != "/tmp" {
		t.Fatalf("expected workdir=/tmp, got %v", m["workdir"])
	}
	if _, ok := m["working_dir"]; ok {
		t.Fatal("expected working_dir to be removed")
	}
	if m["command"] != "pwd" {
		t.Fatalf("expected command=pwd, got %v", m["command"])
	}
}
