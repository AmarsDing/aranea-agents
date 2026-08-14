package tool

import "testing"

func TestCatalogRuntimeStatus(t *testing.T) {
	disabled := Tool{Key: "read_file", Enabled: false, Source: "builtin"}
	if got := toolRuntimeStatus(disabled, nil, nil); got != RuntimeStatusDisabled {
		t.Fatalf("disabled: got %q", got)
	}
	avail := Tool{Key: "read_file", Enabled: true, Source: "builtin"}
	if got := toolRuntimeStatus(avail, nil, nil); got != RuntimeStatusAvailable {
		t.Fatalf("available: got %q", got)
	}
	mcp := Tool{Key: "mcp_call", Enabled: true, Source: "mcp"}
	if got := toolRuntimeStatus(mcp, nil, nil); got != RuntimeStatusRegisteredOnly {
		t.Fatalf("mcp: got %q", got)
	}
	// D5: workspace_exec 运行时尚未实现（装配路径强制关闭），catalog 不得报 available。
	wsExec := Tool{Key: "workspace_exec", Enabled: true, Source: "builtin"}
	if got := toolRuntimeStatus(wsExec, nil, nil); got != RuntimeStatusRegisteredOnly {
		t.Fatalf("workspace_exec: got %q, want registered_only", got)
	}
}

func TestCatalogRuntimeKind(t *testing.T) {
	tool := Tool{RequiresConfirmation: true}
	if got := toolRuntimeKind(tool); got != RuntimeKindApproval {
		t.Fatalf("approval: got %q", got)
	}
}
