package custom

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/toolspill"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// ---------------------------------------------------------------------------
// P0-2 spill:// locator branch of read_tool_result (design §3.8④): parse +
// validate always; the store enforces workspace containment and session
// ownership; unwired deployments answer BadRequest, never a silent nil.
// ---------------------------------------------------------------------------

type fakeBlobReader struct {
	blob *biz.ToolResultBlob
	err  error
}

func (f fakeBlobReader) GetBlob(context.Context, string) (*biz.ToolResultBlob, error) {
	return f.blob, f.err
}
func (f fakeBlobReader) ListBlobsBySession(context.Context, string, int) ([]*biz.ToolResultBlob, error) {
	return nil, nil
}

func newSpillStore(t *testing.T) *toolspill.LocalFS {
	t.Helper()
	store, err := toolspill.NewLocalFS(toolspill.Options{
		WorkspaceRoot: t.TempDir(),
		Logger:        loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewLocalFS: %v", err)
	}
	return store
}

func readCtx(sessionID string) context.Context {
	inv := &trpcagent.Invocation{Session: &trpcsession.Session{ID: sessionID}}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func callReadTool(t *testing.T, tool interface {
	Call(context.Context, []byte) (any, error)
}, ctx context.Context, blobID string, extra map[string]any) (readToolResultOutput, error) {
	t.Helper()
	args := map[string]any{"blob_id": blobID}
	for k, v := range extra {
		args[k] = v
	}
	raw, _ := json.Marshal(args)
	res, err := tool.Call(ctx, raw)
	if err != nil {
		return readToolResultOutput{}, err
	}
	out, ok := res.(readToolResultOutput)
	if !ok {
		t.Fatalf("Call result type = %T, want readToolResultOutput", res)
	}
	return out, nil
}

func mustBeBadRequest(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want BadRequest error, got nil")
	}
	var ae *apierror.Error
	if !errors.As(err, &ae) || ae.Code != apierror.CodeBadRequest {
		t.Fatalf("error = %v, want apierror BadRequest", err)
	}
}

func TestReadToolResult_SpillLocatorUnwiredAnswersBadRequest(t *testing.T) {
	tool := NewReadToolResultTool(fakeBlobReader{}) // no opts: batch-A default
	locator := "spill://fs/" + toolspill.SessionDirName("sess-1") + "/" + strings.Repeat("ab", 16) + ".spill.json"
	err := callReadToolExpectErr(t, tool, readCtx("sess-1"), locator)
	mustBeBadRequest(t, err)
	if !strings.Contains(err.Error(), "not wired") {
		t.Fatalf("unwired locator must be an explicit BadRequest (never silent), got %q", err.Error())
	}
}

func callReadToolExpectErr(t *testing.T, tool interface {
	Call(context.Context, []byte) (any, error)
}, ctx context.Context, blobID string) error {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"blob_id": blobID})
	_, err := tool.Call(ctx, raw)
	return err
}

func TestReadToolResult_SpillLocatorShapeValidation(t *testing.T) {
	store := newSpillStore(t)
	tool := NewReadToolResultTool(fakeBlobReader{}, &ReadToolResultOptions{Spill: store, Logger: loggateway.NewNoop()})
	sha := toolspill.SessionDirName("sess-1")
	bad := []string{
		"spill://",
		"spill://FS/x",
		"spill://fs/../x",
		"spill://fs/a\\b",
		"spill://fs/" + sha + "/../../etc/passwd",                      // workspace escape attempt
		"spill://fs/C:/Windows/win.ini",                                // drive-letter attempt
		"spill://fs//server/share",                                     // UNC-ish attempt
		"spill://fs/" + sha + "/" + strings.Repeat("ab", 16) + ".json", // wrong suffix
	}
	for _, blobID := range bad {
		if err := callReadToolExpectErr(t, tool, readCtx("sess-1"), blobID); err == nil {
			t.Errorf("malformed/traversal locator %q must be rejected", blobID)
		} else {
			var ae *apierror.Error
			if !errors.As(err, &ae) || ae.Code != apierror.CodeBadRequest {
				t.Errorf("locator %q error = %v, want BadRequest", blobID, err)
			}
		}
	}
}

func TestReadToolResult_SpillRoundTripWithPagination(t *testing.T) {
	store := newSpillStore(t)
	payload := strings.Repeat("数据-data-", 4000) // 40K runes > 2 chunks
	loc, err := store.Put(context.Background(), toolspill.Record{
		SessionID: "sess-1",
		Tool:      "web_search",
		Content:   []byte(payload),
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	tool := NewReadToolResultTool(fakeBlobReader{}, &ReadToolResultOptions{Spill: store, Logger: loggateway.NewNoop()})

	ctx := readCtx("sess-1")
	first, err := callReadTool(t, tool, ctx, loc.String(), map[string]any{"limit": 15000})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !first.Found {
		t.Fatal("spilled record must be found for its owning session")
	}
	if first.TotalSize != len([]rune(payload)) {
		t.Fatalf("total_size = %d, want %d", first.TotalSize, len([]rune(payload)))
	}
	if !first.HasMore {
		t.Fatal("first chunk must report has_more")
	}
	second, err := callReadTool(t, tool, ctx, loc.String(), map[string]any{"offset": 15000, "limit": 15000})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if string([]rune(first.Content))+string([]rune(second.Content)) != payload[:len(string([]rune(first.Content)))+len(string([]rune(second.Content)))] {
		// Compare rune-wise below; this cheap check guards gross slicing bugs.
		t.Fatal("chunk concatenation drifted")
	}
	combined := first.Content + second.Content
	if combined != string([]rune(payload)[:30000]) {
		t.Fatalf("paginated content mismatch: got %d runes", len([]rune(combined)))
	}
}

func TestReadToolResult_SpillCrossSessionDenied(t *testing.T) {
	store := newSpillStore(t)
	loc, err := store.Put(context.Background(), toolspill.Record{SessionID: "owner", Content: []byte("mine")})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	tool := NewReadToolResultTool(fakeBlobReader{}, &ReadToolResultOptions{Spill: store, Logger: loggateway.NewNoop()})
	out, err := callReadTool(t, tool, readCtx("intruder"), loc.String(), nil)
	if err != nil {
		t.Fatalf("cross-session read must be Found:false (not an error), got %v", err)
	}
	if out.Found {
		t.Fatal("cross-session read must be denied with Found:false")
	}
	if out.Content != "" {
		t.Fatal("denied read must leak no content")
	}
}

func TestReadToolResult_SpillUnsupportedStoreKind(t *testing.T) {
	store := newSpillStore(t)
	tool := NewReadToolResultTool(fakeBlobReader{}, &ReadToolResultOptions{Spill: store, Logger: loggateway.NewNoop()})
	err := callReadToolExpectErr(t, tool, readCtx("sess-1"), "spill://pg/blob-123")
	mustBeBadRequest(t, err)
	if !strings.Contains(err.Error(), "migration") && !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("unsupported store must surface the migration hint via BadRequest, got %q", err.Error())
	}
}

func TestReadToolResult_BlobPathUnchanged(t *testing.T) {
	// The classic blob path must be unaffected by the new branch and options.
	blob := &biz.ToolResultBlob{ID: "b1", SessionID: "sess-1", FullContent: strings.Repeat("q", 50)}
	tool := NewReadToolResultTool(fakeBlobReader{blob: blob})
	out, err := callReadTool(t, tool, readCtx("sess-1"), "b1", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !out.Found || out.TotalSize != 50 || out.Content != strings.Repeat("q", 50) {
		t.Fatalf("blob path drifted: %+v", out)
	}
	// Cross-session ownership rule intact.
	out, err = callReadTool(t, tool, readCtx("other"), "b1", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out.Found {
		t.Fatal("cross-session blob read must stay denied")
	}
}
