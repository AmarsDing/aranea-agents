package callbacks

import (
	"context"
	"testing"
)

// P1-3: Reject must map to CustomResult (short-circuit tool result), never
// to an error — a user/policy denial is not an interceptor failure, and the
// error path fires spurious "Before tool callback failed" Errorf logs.
func TestDecision_RejectMapsToCustomResult(t *testing.T) {
	res := Reject("denied by user").BeforeToolResult(context.Background())
	if res == nil || res.CustomResult != "denied by user" {
		t.Fatalf("Reject CustomResult = %v, want %q", res, "denied by user")
	}
	if res.ModifiedArguments != nil {
		t.Fatal("Reject must not set ModifiedArguments")
	}
}

// P1-3: Rewrite must map to ModifiedArguments — the framework's only
// write-back channel to toolCall.Function.Arguments.
func TestDecision_RewriteMapsToModifiedArguments(t *testing.T) {
	res := RewriteArgs([]byte(`{"a":1}`)).BeforeToolResult(context.Background())
	if res == nil || string(res.ModifiedArguments) != `{"a":1}` {
		t.Fatalf("Rewrite ModifiedArguments = %v", res)
	}
	if res.CustomResult != nil {
		t.Fatal("Rewrite must not set CustomResult")
	}
}

func TestDecision_PassKeepsContextOnly(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{ k string }{"k"}, "v")
	res := Pass().BeforeToolResult(ctx)
	if res == nil || res.Context != ctx {
		t.Fatal("Pass must carry the context")
	}
	if res.CustomResult != nil || res.ModifiedArguments != nil {
		t.Fatal("Pass must not set CustomResult or ModifiedArguments")
	}
}
