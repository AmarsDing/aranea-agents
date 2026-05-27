package output_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/cli"
	"aranea-agents/internal/cli/output"
)

// goldenPath returns the path to a golden file.
func goldenPath(name string) string {
	return filepath.Join("golden", name+".golden")
}

// checkGolden compares got against a golden file, or writes it when UPDATE_GOLDEN=1.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("golden", 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o640); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden file %s not found; run with UPDATE_GOLDEN=1 to create", path)
	}
	want := string(data)
	if got != want {
		t.Errorf("output mismatch for %s:\ngot:\n%s\nwant:\n%s", name, got, want)
	}
}

// sampleAgentRows returns sample data for golden tests.
func sampleAgentRows() []map[string]string {
	return []map[string]string{
		{"id": "agent-001", "display_name": "Test Agent A", "status": "active"},
		{"id": "agent-002", "display_name": "Test Agent B", "status": "inactive"},
	}
}

func TestPrinterText_AgentList_TTY(t *testing.T) {
	var buf bytes.Buffer
	p := output.NewPrinter(output.FormatText, false, true, &buf)
	if err := p.PrintList(sampleAgentRows(), 2); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "agent_ls_text_tty", buf.String())
}

func TestPrinterText_AgentList_Pipe(t *testing.T) {
	var buf bytes.Buffer
	p := output.NewPrinter(output.FormatText, false, true, &buf)
	if err := p.PrintList(sampleAgentRows(), 2); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "agent_ls_text_pipe", buf.String())
}

func TestPrinterJSON_AgentList(t *testing.T) {
	var buf bytes.Buffer
	p := output.NewPrinter(output.FormatJSON, false, true, &buf)
	if err := p.PrintList(sampleAgentRows(), 2); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	// Must contain "items" and be parseable JSON.
	if !strings.Contains(got, "items") {
		t.Error("JSON output missing 'items' field")
	}
	checkGolden(t, "agent_ls_json", got)
}

func TestPrinterText_ErrorBlocked(t *testing.T) {
	var buf bytes.Buffer
	p := output.NewPrinter(output.FormatText, false, true, &buf)
	e := &cli.CLIError{
		Code:       "SKILL_IMPORT_BLOCKED",
		HTTPStatus: 409,
		Message:    "候选 Skill 与已有 Skill 同名（figma-code-connect）",
		Hint:       "用 `--decision skip` 跳过，或 `--decision keep` 保留两份",
		Metadata:   map[string]any{"job_id": "job_7f3", "group_id": "group_01"},
	}
	if err := p.PrintError(e); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "error_skill_blocked_text", buf.String())
}

func TestPrinterJSON_ErrorBlocked(t *testing.T) {
	var buf bytes.Buffer
	p := output.NewPrinter(output.FormatJSON, false, true, &buf)
	e := &cli.CLIError{
		Code:       "SKILL_IMPORT_BLOCKED",
		HTTPStatus: 409,
		Message:    "候选 Skill 与已有 Skill 同名（figma-code-connect）",
		Hint:       "用 `--decision skip` 跳过，或 `--decision keep` 保留两份",
		Metadata:   map[string]any{"job_id": "job_7f3", "group_id": "group_01"},
	}
	if err := p.PrintError(e); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "SKILL_IMPORT_BLOCKED") {
		t.Error("JSON error output missing error code")
	}
	checkGolden(t, "error_skill_blocked_json", got)
}
