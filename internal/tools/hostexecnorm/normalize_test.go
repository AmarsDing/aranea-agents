package hostexecnorm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeExecArgs_workingDirAlias(t *testing.T) {
	t.Parallel()
	out := NormalizeExecArgs([]byte(`{"command":"pwd","working_dir":"sub"}`))
	if string(out) != `{"command":"pwd","workdir":"sub"}` {
		t.Fatalf("got %s", out)
	}
}

func TestNormalizeExecArgs_preservesWorkdir(t *testing.T) {
	t.Parallel()
	in := []byte(`{"command":"pwd","workdir":"sub"}`)
	if string(NormalizeExecArgs(in)) != string(in) {
		t.Fatal("expected unchanged when workdir present")
	}
}

func TestNormalizeExecArgs_CmdAlias(t *testing.T) {
	out := NormalizeExecArgs([]byte(`{"cmd":"dir"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["command"] != "dir" {
		t.Fatalf("command = %v", m["command"])
	}
	if _, ok := m["cmd"]; ok {
		t.Fatal("cmd alias should be removed")
	}
}

func TestNormalizeExecArgs_CwdAlias(t *testing.T) {
	out := NormalizeExecArgs([]byte(`{"command":"pwd","cwd":"sub"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["workdir"] != "sub" {
		t.Fatalf("workdir = %v", m["workdir"])
	}
}

func TestNormalizeExecArgs_TimeoutAlias(t *testing.T) {
	out := NormalizeExecArgs([]byte(`{"command":"sleep 1","timeout":30}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["timeout_sec"] != float64(30) {
		t.Fatalf("timeout_sec = %v", m["timeout_sec"])
	}
	if _, ok := m["timeout"]; ok {
		t.Fatal("timeout alias should be removed")
	}
}

func TestNormalizeExecArgs_TimeoutStringAlias(t *testing.T) {
	out := NormalizeExecArgs([]byte(`{"command":"sleep 1","timeout":"30"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["timeout_sec"] != float64(30) {
		t.Fatalf("timeout_sec = %v", m["timeout_sec"])
	}
}

func TestNormalizeExecArgs_PreservesCommandOverCmd(t *testing.T) {
	in := []byte(`{"command":"pwd","cmd":"dir"}`)
	if string(NormalizeExecArgs(in)) != string(in) {
		t.Fatal("expected unchanged when command already present")
	}
}
