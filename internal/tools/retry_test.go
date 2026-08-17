package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type retryNetError struct {
	timeout   bool
	temporary bool
}

func (e retryNetError) Error() string   { return "retry net error" }
func (e retryNetError) Timeout() bool   { return e.timeout }
func (e retryNetError) Temporary() bool { return e.temporary }

func TestIsRetryableTool(t *testing.T) {
	retryable := []string{"read_file", "list_file", "web_fetch", "httpfetch", "duckduckgo_search"}
	for _, name := range retryable {
		if !IsRetryableTool(name) {
			t.Errorf("IsRetryableTool(%q) = false, want true", name)
		}
	}
	notRetryable := []string{
		"", "exec_command", "write_stdin", "save_file", "diff_edit",
		"write_file", "send_email", "unknown_mutating_tool",
	}
	for _, name := range notRetryable {
		if IsRetryableTool(name) {
			t.Errorf("IsRetryableTool(%q) = true, want false", name)
		}
	}
}

func TestSelectiveRetryOn_RetriesTransientRead(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "read_file",
		Error:    io.ErrUnexpectedEOF,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Fatal("read_file unexpected EOF should retry")
	}

	retry, err = SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "web_fetch",
		Error:    retryNetError{timeout: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Fatal("web_fetch timeout should retry")
	}
}

func TestSelectiveRetryOn_DoesNotRetryExclusiveOrWrites(t *testing.T) {
	cases := []string{"exec_command", "save_file", "send_email"}
	for _, name := range cases {
		retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
			ToolName: name,
			Error:    retryNetError{timeout: true},
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", name, err)
		}
		if retry {
			t.Errorf("%s must not retry even on timeout", name)
		}
	}
}

func TestSelectiveRetryOn_DoesNotRetryNonTransient(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "read_file",
		Error:    errors.New("not found"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Fatal("non-transient read_file error must not retry")
	}
}

func TestSelectiveRetryOn_NilInfo(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Fatal("nil info must not retry")
	}
}

func TestSelectiveRetryOn_RetriesWebFetchResultHTTP503(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "web_fetch",
		Result: map[string]any{
			"results": []any{
				map[string]any{"status_code": 503, "error": "HTTP status 503"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Fatal("web_fetch HTTP 503 result should retry")
	}
}

func TestSelectiveRetryOn_DoesNotRetryWebFetchHTTP404(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "web_fetch",
		Result: map[string]any{
			"results": []any{
				map[string]any{"status_code": 404, "error": "HTTP status 404"},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Fatal("web_fetch HTTP 404 must not retry")
	}
}

func TestSelectiveRetryOn_DoesNotRetryExecCommandResult503(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "exec_command",
		Result: map[string]any{
			"status_code": 503,
			"error":       "HTTP status 503",
		},
		Error: errors.New("HTTP status 503"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retry {
		t.Fatal("exec_command must not retry even on a 503-shaped result")
	}
}

func TestSelectiveRetryOn_RetriesWrappedDuckDuckGoTimeout(t *testing.T) {
	// duckduckgo wraps the transport error with %v (not %w), so DefaultRetryOn
	// cannot unwrap net.Error / EOF.
	wrapped := fmt.Errorf("error performing search: %v", errors.New("Get \"https://example.com\": i/o timeout"))
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "duckduckgo_search",
		Error:    wrapped,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Fatal("duckduckgo_search wrapped timeout without unwrap should retry")
	}
}

func TestSelectiveRetryOn_RetriesFlaggedResultError(t *testing.T) {
	flagged := FlagTransientResult("web_fetch", map[string]any{"status_code": 503, "error": "HTTP status 503"})
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName:    "web_fetch",
		Result:      flagged,
		ResultError: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Fatal("DefaultRetryOn refuses ResultError; SelectiveRetryOn must still retry flagged 503")
	}
}

func TestSelectiveRetryOn_RetriesWebFetch429(t *testing.T) {
	retry, err := SelectiveRetryOn(context.Background(), &trpctool.RetryInfo{
		ToolName: "web_fetch",
		Result:   map[string]any{"status_code": 429, "error": "HTTP status 429"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !retry {
		t.Fatal("web_fetch HTTP 429 should retry")
	}
}

func TestFlagTransientResult_MarksWebFetch503ForRetryRunner(t *testing.T) {
	result := FlagTransientResult("web_fetch", map[string]any{
		"results": []any{
			map[string]any{"status_code": 503, "error": "HTTP status 503"},
		},
	})
	flagged, ok := result.(retryFlaggedResult)
	if !ok {
		t.Fatal("web_fetch 503 must be flagged so the retry runner does not treat it as success")
	}
	if !flagged.RetryResultError() {
		t.Fatal("flagged result must implement RetryResultError() == true")
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !strings.Contains(string(raw), `"status_code":503`) && !strings.Contains(string(raw), `"status_code": 503`) {
		t.Fatalf("JSON should unwrap to original payload, got %s", raw)
	}
}

func TestFlagTransientResult_DoesNotFlag404OrWrites(t *testing.T) {
	if _, ok := FlagTransientResult("web_fetch", map[string]any{"status_code": 404}).(retryFlaggedResult); ok {
		t.Fatal("HTTP 404 must not be flagged")
	}
	if _, ok := FlagTransientResult("exec_command", map[string]any{"status_code": 503}).(retryFlaggedResult); ok {
		t.Fatal("exec_command must not be flagged")
	}
	if _, ok := FlagTransientResult("save_file", map[string]any{"error": "timeout"}).(retryFlaggedResult); ok {
		t.Fatal("save_file must not be flagged")
	}
}
