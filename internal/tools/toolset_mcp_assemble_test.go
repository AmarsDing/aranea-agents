package tools

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
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
	}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("buildMCPBrokerTools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected broker tools")
	}
}

type captureWarnLogger struct{ warns int }

func (l *captureWarnLogger) Debug(string, ...loggateway.Field)          {}
func (l *captureWarnLogger) Info(string, ...loggateway.Field)           {}
func (l *captureWarnLogger) Warn(string, ...loggateway.Field)           { l.warns++ }
func (l *captureWarnLogger) Error(string, ...loggateway.Field)          {}
func (l *captureWarnLogger) With(...loggateway.Field) loggateway.Logger { return l }

// TestBuildMCPBrokerTools_StdioEnvDropWarns pins 2026-11-03 trpc 出库决策：
// broker 托管的 stdio 服务器 Env 被上游 mcpbroker 丢弃时不允许静默——必须
// 打 Warn；HTTP 传输的 Env 本就无意义，不告警；nil logger 不 panic。
func TestBuildMCPBrokerTools_StdioEnvDropWarns(t *testing.T) {
	lg := &captureWarnLogger{}
	_, err := buildMCPBrokerTools(MCPBrokerConfig{
		Servers: []MCPServerConfig{
			{Name: "stdio-env", Transport: "stdio", Command: "false", Env: map[string]string{"FOO": "bar"}},
			{Name: "stdio-noenv", Transport: "stdio", Command: "false"},
			{Name: "http-env", Transport: "streamable", ServerURL: "http://127.0.0.1:1/mcp", Env: map[string]string{"X": "y"}},
		},
	}, lg)
	if err != nil {
		t.Fatalf("buildMCPBrokerTools: %v", err)
	}
	if lg.warns != 1 {
		t.Fatalf("warn count = %d, want 1 (only stdio+Env warns)", lg.warns)
	}
	// nil logger 不 panic。
	if _, err := buildMCPBrokerTools(MCPBrokerConfig{
		Servers: []MCPServerConfig{{Name: "stdio-env", Transport: "stdio", Command: "false", Env: map[string]string{"A": "b"}}},
	}, nil); err != nil {
		t.Fatalf("nil logger buildMCPBrokerTools: %v", err)
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
