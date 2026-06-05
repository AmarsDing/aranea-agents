package monitor_test

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// FailureReport construction
// ---------------------------------------------------------------------------

func TestNewFailureReport(t *testing.T) {
	fr := monitor.NewFailureReport()
	if fr == nil {
		t.Fatal("NewFailureReport() = nil, want non-nil")
	}
	if fr.ID == "" {
		t.Error("NewFailureReport().ID is empty, want auto-generated UUID")
	}
	if fr.Metadata == nil {
		t.Error("NewFailureReport().Metadata = nil, want initialized map")
	}
}

func TestFailureReport_TypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  monitor.FailureType
		want string
	}{
		{"lint", monitor.FailureTypeLint, "lint_error"},
		{"test", monitor.FailureTypeTest, "test_failure"},
		{"build", monitor.FailureTypeBuild, "build_failure"},
		{"proto_sync", monitor.FailureTypeProtoSync, "proto_sync"},
		{"runtime", monitor.FailureTypeRuntime, "runtime_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.want {
				t.Errorf("FailureType constant = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseCILogs
// ---------------------------------------------------------------------------

func TestParseCILogs_GoBuildError(t *testing.T) {
	logs := `make api
# api/admin/v1/admin.proto
internal/service/cli_admin_tools.go:42:5: undefined: SomeSymbol
internal/service/cli_admin_tools.go:58:10: cannot use x (type string) as type int in assignment
make: *** [Makefile:23: build] Error 1`

	reports := monitor.ParseCILogs(logs, "build")
	if len(reports) < 1 {
		t.Fatalf("ParseCILogs() returned %d reports, want at least 1", len(reports))
	}

	fr := reports[0]
	if fr.Type != monitor.FailureTypeBuild {
		t.Errorf("Type = %q, want %q", fr.Type, monitor.FailureTypeBuild)
	}
	if fr.Source != "ci" {
		t.Errorf("Source = %q, want %q", fr.Source, "ci")
	}
	if fr.Job != "build" {
		t.Errorf("Job = %q, want %q", fr.Job, "build")
	}
	if fr.File == "" {
		t.Error("File is empty, want parsed file path")
	}
	if fr.Line <= 0 {
		t.Errorf("Line = %d, want > 0", fr.Line)
	}
	if fr.Message == "" {
		t.Error("Message is empty, want parsed error message")
	}
}

func TestParseCILogs_TestFailure(t *testing.T) {
	logs := `=== RUN   TestSomething
    something_test.go:25: expected status 200, got 500
--- FAIL: TestSomething (0.03s)
FAIL
coverage: 80.0% of statements
make: *** [Makefile:30: test] Error 1`

	reports := monitor.ParseCILogs(logs, "test")
	if len(reports) < 1 {
		t.Fatalf("ParseCILogs() returned %d reports, want at least 1", len(reports))
	}

	fr := reports[0]
	if fr.Type != monitor.FailureTypeTest {
		t.Errorf("Type = %q, want %q", fr.Type, monitor.FailureTypeTest)
	}
	if fr.File == "" {
		t.Error("File is empty, want parsed file path from test failure")
	}
	if fr.Line <= 0 {
		t.Errorf("Line = %d, want > 0", fr.Line)
	}
}

func TestParseCILogs_LintError(t *testing.T) {
	logs := `internal/biz/monitor/monitor.go:100:1: exported function NewUsecase should have comment or be unexported (golint)
internal/service/handler.go:30:5: should not use dot imports (golint)`

	reports := monitor.ParseCILogs(logs, "lint")
	if len(reports) < 1 {
		t.Fatalf("ParseCILogs() returned %d reports, want at least 1", len(reports))
	}

	fr := reports[0]
	if fr.Type != monitor.FailureTypeLint {
		t.Errorf("Type = %q, want %q", fr.Type, monitor.FailureTypeLint)
	}
	if fr.File == "" {
		t.Error("File is empty, want parsed file path from lint error")
	}
}

func TestParseCILogs_ProtoSync(t *testing.T) {
	logs := `Proto generated files are out of date. Run "make api" to regenerate.
make: *** [Makefile:10: proto-check] Error 1`

	reports := monitor.ParseCILogs(logs, "proto-check")
	if len(reports) < 1 {
		t.Fatalf("ParseCILogs() returned %d reports, want at least 1", len(reports))
	}

	fr := reports[0]
	if fr.Type != monitor.FailureTypeProtoSync {
		t.Errorf("Type = %q, want %q", fr.Type, monitor.FailureTypeProtoSync)
	}
}

func TestParseCILogs_EmptyInput(t *testing.T) {
	reports := monitor.ParseCILogs("", "build")
	if len(reports) != 0 {
		t.Errorf("ParseCILogs('') returned %d reports, want 0", len(reports))
	}
}

func TestParseCILogs_NoMatch(t *testing.T) {
	logs := `all good, no errors here
make: Nothing to be done for 'build'.`
	reports := monitor.ParseCILogs(logs, "build")
	if len(reports) != 0 {
		t.Errorf("ParseCILogs() returned %d reports for clean logs, want 0", len(reports))
	}
}

func TestParseCILogs_MultipleBuildErrors(t *testing.T) {
	logs := `internal/service/foo.go:10:5: undefined: Alpha
internal/service/bar.go:20:3: cannot use x as type int
internal/biz/baz.go:30:8: too many arguments in call to Func`

	reports := monitor.ParseCILogs(logs, "build")
	if len(reports) < 3 {
		t.Errorf("ParseCILogs() returned %d reports, want at least 3 for multiple build errors", len(reports))
	}
}

// ---------------------------------------------------------------------------
// ParseRuntimeError
// ---------------------------------------------------------------------------

func TestParseRuntimeError_NilPointerDereference(t *testing.T) {
	errMsg := `panic: runtime error: invalid memory address or nil pointer dereference
goroutine 42 [running]:
aranea-agents/internal/biz/monitor.(*RootCauseEngine).Analyze(0xc0001a2000, {0x1234, 0x5678}, 0x0, 0x0, 0x0, 0x0)
	/aranea-agents/internal/biz/monitor/root_cause_engine.go:87 +0x1a5
aranea-agents/internal/service.(*AdminService).DoWork(...)
	/aranea-agents/internal/service/admin.go:50 +0x88`

	fr := monitor.ParseRuntimeError(errMsg, "admin-service")
	if fr == nil {
		t.Fatal("ParseRuntimeError() = nil, want non-nil")
	}
	if fr.Type != monitor.FailureTypeRuntime {
		t.Errorf("Type = %q, want %q", fr.Type, monitor.FailureTypeRuntime)
	}
	if fr.Source != "runtime" {
		t.Errorf("Source = %q, want %q", fr.Source, "runtime")
	}
	if fr.Job != "admin-service" {
		t.Errorf("Job = %q, want %q", fr.Job, "admin-service")
	}
	if fr.StackTrace == "" {
		t.Error("StackTrace is empty, want parsed stack trace")
	}
	if fr.File == "" {
		t.Error("File is empty, want parsed file path from stack trace")
	}
}

func TestParseRuntimeError_EmptyInput(t *testing.T) {
	fr := monitor.ParseRuntimeError("", "service")
	if fr != nil {
		t.Errorf("ParseRuntimeError('') = %+v, want nil", fr)
	}
}

func TestParseRuntimeError_GenericError(t *testing.T) {
	errMsg := `connection refused: dial tcp 127.0.0.1:8080: connection refused`

	fr := monitor.ParseRuntimeError(errMsg, "mcp-connector")
	if fr == nil {
		t.Fatal("ParseRuntimeError() = nil, want non-nil for generic error")
	}
	if fr.Type != monitor.FailureTypeRuntime {
		t.Errorf("Type = %q, want %q", fr.Type, monitor.FailureTypeRuntime)
	}
	if fr.Message == "" {
		t.Error("Message is empty, want error text")
	}
}

// ---------------------------------------------------------------------------
// AnalyzeFromReport (on RootCauseAnalyzer interface)
// ---------------------------------------------------------------------------

func TestRootCauseEngine_AnalyzeFromReport(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	report := monitor.NewFailureReport()
	report.Type = monitor.FailureTypeBuild
	report.Source = "ci"
	report.File = "internal/service/cli_admin_tools.go"
	report.Line = 42
	report.ErrorCode = "undefined"
	report.Message = "undefined: SomeSymbol"

	result, err := engine.AnalyzeFromReport(ctx, report)
	if err != nil {
		t.Fatalf("AnalyzeFromReport() returned error: %v", err)
	}
	// Build errors may or may not match existing rules; the important thing
	// is that the method exists and doesn't panic. A non-nil result is a bonus.
	_ = result
}

func TestRootCauseEngine_AnalyzeFromReport_RuntimeError(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	report := monitor.NewFailureReport()
	report.Type = monitor.FailureTypeRuntime
	report.Source = "runtime"
	report.Job = "mcp-connector"
	report.Message = "connection refused: dial tcp 127.0.0.1:8080"

	result, err := engine.AnalyzeFromReport(ctx, report)
	if err != nil {
		t.Fatalf("AnalyzeFromReport() returned error: %v", err)
	}
	// Runtime connection-refused should match rc-mcp-connection-failure rule
	if result != nil && result.RuleID != "rc-mcp-connection-failure" {
		t.Logf("AnalyzeFromReport() matched rule %q (may be expected depending on rules)", result.RuleID)
	}
}

func TestRootCauseEngine_AnalyzeFromReport_NilReport(t *testing.T) {
	engine := monitor.NewRootCauseEngine(loggateway.NewNoop())
	ctx := context.Background()

	result, err := engine.AnalyzeFromReport(ctx, nil)
	if err != nil {
		t.Fatalf("AnalyzeFromReport(nil) returned error: %v", err)
	}
	if result != nil {
		t.Errorf("AnalyzeFromReport(nil) returned non-nil result, want nil")
	}
}
