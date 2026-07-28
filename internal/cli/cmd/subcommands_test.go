package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	skillv1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/client"
	"aranea-agents/internal/cli/output"
	"github.com/spf13/cobra"
)

// writeTestFile writes content to path for test fixtures.
func writeTestFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// assertSubcommands verifies that all names are registered on the parent command.
func assertSubcommands(t *testing.T, parent *cobra.Command, names ...string) {
	t.Helper()
	registered := map[string]bool{}
	for _, sub := range parent.Commands() {
		registered[sub.Name()] = true
	}
	for _, name := range names {
		if !registered[name] {
			t.Errorf("subcommand %q not registered on %q", name, parent.Name())
		}
	}
}

func TestSessionCmd_NewSubcommands(t *testing.T) {
	assertSubcommands(t, NewSessionCmd(), "archive", "restore", "pin", "unpin", "compact", "export")
}

func TestSkillCmd_NewSubcommands(t *testing.T) {
	assertSubcommands(t, NewSkillCmd(), "files", "file-get", "file-put", "file-delete", "import")
}

func TestCronCmd_ResetFailures(t *testing.T) {
	assertSubcommands(t, NewCronCmd(), "reset-failures")
}

func TestMCPCmd_Validate(t *testing.T) {
	assertSubcommands(t, NewMCPCmd(), "validate")
}

func TestToolCmd_Test(t *testing.T) {
	assertSubcommands(t, NewToolCmd(), "test")
}

func TestBuildImportDecisions(t *testing.T) {
	job := &skillv1.SkillImportJob{
		JobId: "job-1",
		Candidates: []*skillv1.SkillImportCandidate{
			{CandidateId: "c-pass", ValidationStatus: "pass"},
			{CandidateId: "c-warn", ValidationStatus: "warn"},
			{CandidateId: "c-blocked", ValidationStatus: "pass",
				Blocks: []*skillv1.SkillImportIssue{{Type: "duplicate_name", Message: "dup"}}},
		},
	}
	decisions, pending := buildImportDecisions(job)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].CandidateId != "c-pass" || decisions[0].Action != "import_passed" {
		t.Errorf("unexpected decision: %+v", decisions[0])
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d: %v", len(pending), pending)
	}
}

// newTestCLIContext builds a cli.Context backed by the given test server.
func newTestCLIContext(srvURL string, printerOut *bytes.Buffer) *cli.Context {
	return &cli.Context{
		Client:  client.NewClient(srvURL, "tok", "dev", false, nil),
		Printer: output.NewPrinter(output.FormatText, false, true, printerOut),
		AutoYes: true,
	}
}

func TestSessionExportCmd_Stdout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sessions/sess-1/export" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":"# Exported","filename":"sess-1.md","content_type":"text/markdown"}`))
	}))
	defer srv.Close()

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	cmd := sessionExportCmd()
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"sess-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("export: %v", err)
	}
	if out.String() != "# Exported" {
		t.Errorf("expected exported content on stdout, got %q", out.String())
	}
}

func TestSessionExportCmd_InvalidFormat(t *testing.T) {
	cmd := sessionExportCmd()
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"sess-1", "--format", "yaml"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != "INVALID_FORMAT" {
		t.Fatalf("expected CLIError INVALID_FORMAT, got %v", err)
	}
}

func TestToolTestCmd_RunE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tools/tool-1/test" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","result_preview":"ok","duration_ms":7}`))
	}))
	defer srv.Close()

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	cmd := toolTestCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"tool-1", "--args", `{"q":"hi"}`})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tool test: %v", err)
	}
	if !bytes.Contains(printerOut.Bytes(), []byte("success")) {
		t.Errorf("expected printer output to contain status, got %q", printerOut.String())
	}
}

func TestCronResetFailuresCmd_RunE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/cron-tasks/ct-1/reset-failures" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ct-1","status":"active"}`))
	}))
	defer srv.Close()

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	cmd := cronResetFailuresCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"ct-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cron reset-failures: %v", err)
	}
}

func TestMCPValidateCmd_RunE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/mcp-servers/validate" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"status":"valid"}`))
	}))
	defer srv.Close()

	cfgFile := t.TempDir() + "/mcp.json"
	if err := writeTestFile(cfgFile, `{"command":"npx"}`); err != nil {
		t.Fatal(err)
	}

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	cmd := mcpValidateCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"--file", cfgFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("mcp validate: %v", err)
	}
}

func TestMCPValidateCmd_FailureExitNonZero(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"status":"invalid","message":"bad transport"}`))
	}))
	defer srv.Close()

	cfgFile := t.TempDir() + "/mcp.json"
	if err := writeTestFile(cfgFile, `{"command":"npx"}`); err != nil {
		t.Fatal(err)
	}

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	cmd := mcpValidateCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--file", cfgFile})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("validation failure must return an error, got nil")
	}
	var ce *cli.CLIError
	if !errors.As(err, &ce) || ce.Code != "VALIDATION_FAILED" {
		t.Fatalf("expected CLIError VALIDATION_FAILED, got %v", err)
	}
	if code := cli.ExitCodeOf(err); code == cli.ExitOK {
		t.Errorf("validation failure must exit non-zero (CI contract), got %d", code)
	}
}

func TestSkillFilesCmd_RunE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/skills/sk-1/files" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"path":"SKILL.md","name":"SKILL.md","language":"markdown","size":64}]}`))
	}))
	defer srv.Close()

	var printerOut bytes.Buffer
	cc := newTestCLIContext(srv.URL, &printerOut)
	ctx := cli.WithCLI(context.Background(), cc)

	cmd := skillFilesCmd()
	cmd.SetContext(ctx)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"sk-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("skill files: %v", err)
	}
	if !bytes.Contains(printerOut.Bytes(), []byte("SKILL.md")) {
		t.Errorf("expected printer output to contain SKILL.md, got %q", printerOut.String())
	}
}
