package tools

import (
	"context"
	"testing"
)

func TestBuildMCPToolSet_StdioConfig(t *testing.T) {
	ts, err := buildMCPToolSet(MCPServerConfig{
		Name:      "unit-stdio",
		Transport: "stdio",
		Command:   "false",
	})
	if err != nil {
		t.Fatalf("buildMCPToolSet: %v", err)
	}
	if ts == nil {
		t.Fatal("expected non-nil ToolSet")
	}
	if ts.Name() == "" {
		t.Fatal("expected toolset name")
	}
}

func TestBuildMCPBrokerTools_NamedServers(t *testing.T) {
	tools, err := buildMCPBrokerTools(MCPBrokerConfig{
		Servers: []MCPServerConfig{
			{Name: "local", Transport: "stdio", Command: "false"},
		},
		AllowAdHocHTTP: false,
	})
	if err != nil {
		t.Fatalf("buildMCPBrokerTools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected broker tools")
	}
}

func TestAssemble_mcpBrokerAddsTools(t *testing.T) {
	out, err := Assemble(context.Background(), AssemblyConfig{
		EnabledTools: []string{"mcpbroker"},
		MCP: MCPConfig{
			Broker: &MCPBrokerConfig{
				Servers: []MCPServerConfig{
					{Name: "demo", Transport: "stdio", Command: "false"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if len(out.Tools) == 0 {
		t.Fatal("expected mcpbroker tools in assembled output")
	}
}
