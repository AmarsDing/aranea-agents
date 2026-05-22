package hostexecnorm

import "testing"

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
