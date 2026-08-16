package inverse

import "testing"

func TestRegisterAndLookup(t *testing.T) {
	resetForTest()

	if _, ok := LookupForward("ops_inject"); ok {
		t.Fatal("unregistered tool must not have a forward spec")
	}
	if IsInverse("ops_clear") {
		t.Fatal("unregistered inverse tool must not be reported")
	}

	Register("ops_inject", Spec{InverseTool: "ops_clear"})

	spec, ok := LookupForward("ops_inject")
	if !ok || spec.InverseTool != "ops_clear" {
		t.Fatalf("LookupForward = %+v, %v", spec, ok)
	}
	if !IsInverse("ops_clear") {
		t.Fatal("ops_clear must be reported as inverse")
	}
}

func TestRegisterIdempotent(t *testing.T) {
	resetForTest()

	Register("ops_inject", Spec{InverseTool: "ops_clear"})
	Register("ops_inject", Spec{InverseTool: "ops_clear"})

	mu.RLock()
	defer mu.RUnlock()
	if got := len(reverse["ops_clear"]); got != 1 {
		t.Fatalf("reverse index len = %d, want 1 (duplicate registration must be a no-op)", got)
	}
}

func TestRegisterRejectsEmpty(t *testing.T) {
	resetForTest()

	Register("", Spec{InverseTool: "ops_clear"})
	Register("ops_inject", Spec{})

	if _, ok := LookupForward(""); ok {
		t.Fatal("empty tool name must be rejected")
	}
	if _, ok := LookupForward("ops_inject"); ok {
		t.Fatal("empty InverseTool must be rejected")
	}
}
