package agent

import (
	"errors"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestIsMCPCatalogToolName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"mcp_list_tools", "local_stdio_mcp_list_tools", "mcp_list_resources"} {
		if !isMCPCatalogToolName(name) {
			t.Fatalf("%q should trigger refresh", name)
		}
	}
	if isMCPCatalogToolName("shell_exec") || isMCPCatalogToolName("mcp_call") {
		t.Fatal("shell_exec / mcp_call must not always invalidate")
	}
}

func TestShouldRefreshMCPCatalog_UnknownTool(t *testing.T) {
	t.Parallel()
	if !shouldRefreshMCPCatalog(&trpctool.AfterToolArgs{
		ToolName: "mcp_call",
		Error:    errors.New("unknown tool foo"),
	}) {
		t.Fatal("unknown tool error must refresh")
	}
	if shouldRefreshMCPCatalog(&trpctool.AfterToolArgs{
		ToolName: "mcp_call",
		Error:    errors.New("timeout"),
	}) {
		t.Fatal("generic mcp_call errors must not refresh")
	}
}
