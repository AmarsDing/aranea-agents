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
}

// registered_only 以前把「要会话才装配」「缺配置」「真没实现」混在一起，管理页据此显示成 Agent 调不到。
func TestCatalogRuntimeStatusReasons(t *testing.T) {
	cases := []struct {
		name string
		tool Tool
		want string
	}{
		{"session bound", Tool{Key: "memory_search", Enabled: true, Source: "builtin"}, RuntimeStatusSessionBound},
		{"working memory underscore key", Tool{Key: "working_memory_write", Enabled: true, Source: "builtin"}, RuntimeStatusSessionBound},
		{"google search without key", Tool{Key: "google_search", Enabled: true, Source: "builtin"}, RuntimeStatusNeedsConfig},
		{"google search configured", Tool{Key: "google_search", Enabled: true, Source: "builtin",
			ConfigJSON: `{"api_key":"k","cx":"c"}`}, RuntimeStatusAvailable},
		{"deferred registry backed", Tool{Key: "datetime", Enabled: true, Source: "builtin"}, RuntimeStatusAvailable},
		{"effective toolset backed", Tool{Key: ToolKeyMail, Enabled: true, Source: "builtin"}, RuntimeStatusAvailable},
		{"unknown builtin", Tool{Key: "no_such_tool", Enabled: true, Source: "builtin"}, RuntimeStatusRegisteredOnly},
	}
	for _, c := range cases {
		if got := toolRuntimeStatus(c.tool, nil, nil); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestCatalogRuntimeKind(t *testing.T) {
	tool := Tool{RequiresConfirmation: true}
	if got := toolRuntimeKind(tool); got != RuntimeKindApproval {
		t.Fatalf("approval: got %q", got)
	}
}
