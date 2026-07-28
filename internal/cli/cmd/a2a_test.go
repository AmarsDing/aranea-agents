package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/cli"
)

func TestNewA2ACmd_Constructs(t *testing.T) {
	c := NewA2ACmd()
	if c.Use != "a2a" {
		t.Errorf("Use: got %q, want %q", c.Use, "a2a")
	}
	for _, name := range []string{"discover", "remote-agents", "audit", "config"} {
		if findSubCmd(c, name) == nil {
			t.Errorf("missing subcommand %q", name)
		}
	}
	ra := findSubCmd(c, "remote-agents")
	for _, name := range []string{"ls", "get", "add", "delete"} {
		if findSubCmd(ra, name) == nil {
			t.Errorf("missing remote-agents subcommand %q", name)
		}
	}
	if findSubCmd(findSubCmd(c, "audit"), "ls") == nil {
		t.Error("missing audit subcommand \"ls\"")
	}
	if findSubCmd(findSubCmd(c, "config"), "get") == nil {
		t.Error("missing config subcommand \"get\"")
	}
}

func TestA2ADiscoverCmd_MissingURL(t *testing.T) {
	c := a2aDiscoverCmd()
	silenceCmd(c)
	c.SetArgs([]string{})
	if err := c.Execute(); err == nil {
		t.Fatal("expected error for missing --url, got nil")
	}
}

func TestA2ARemoteAgentsAddCmd_MissingURL(t *testing.T) {
	c := a2aRemoteAgentsAddCmd()
	silenceCmd(c)
	c.SetArgs([]string{"--name", "Bot"})
	if err := c.Execute(); err == nil {
		t.Fatal("expected error for missing --url, got nil")
	}
}

func TestA2ARemoteAgentsGetCmd_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/a2a/remote-agents" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"ra-1","displayName":"Bot"}]}`))
	}))
	defer srv.Close()

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	c := a2aRemoteAgentsGetCmd()
	silenceCmd(c)
	c.SetContext(ctx)
	c.SetArgs([]string{"ra-missing"})
	err := c.Execute()
	var ce *cli.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *cli.CLIError, got %T: %v", err, err)
	}
	if ce.Code != "REMOTE_AGENT_NOT_FOUND" {
		t.Errorf("code: got %q, want %q", ce.Code, "REMOTE_AGENT_NOT_FOUND")
	}
}

func TestA2ARemoteAgentsGetCmd_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"ra-1","displayName":"Bot","remoteUrl":"https://a2a.example.com","enabled":true}]}`))
	}))
	defer srv.Close()

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	c := a2aRemoteAgentsGetCmd()
	silenceCmd(c)
	c.SetContext(ctx)
	c.SetArgs([]string{"ra-1"})
	if err := c.Execute(); err != nil {
		t.Fatalf("get: %v", err)
	}
	if !bytes.Contains(printerOut.Bytes(), []byte("ra-1")) {
		t.Errorf("expected printer output to contain ra-1, got %q", printerOut.String())
	}
}
