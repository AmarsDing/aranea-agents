package biz

import "testing"

func TestCatalogRuntimeStatus(t *testing.T) {
	disabled := Tool{Key: "read_file", Enabled: false, Source: "builtin"}
	if got := catalogRuntimeStatus(disabled, nil); got != RuntimeStatusDisabled {
		t.Fatalf("disabled: got %q", got)
	}
	avail := Tool{Key: "read_file", Enabled: true, Source: "builtin"}
	if got := catalogRuntimeStatus(avail, nil); got != RuntimeStatusAvailable {
		t.Fatalf("available: got %q", got)
	}
	mcp := Tool{Key: "mcp_call", Enabled: true, Source: "mcp"}
	if got := catalogRuntimeStatus(mcp, nil); got != RuntimeStatusCatalogOnly {
		t.Fatalf("mcp: got %q", got)
	}
}

func TestCatalogRuntimeKind(t *testing.T) {
	tool := Tool{RequiresConfirmation: true}
	if got := catalogRuntimeKind(tool); got != RuntimeKindApproval {
		t.Fatalf("approval: got %q", got)
	}
}
