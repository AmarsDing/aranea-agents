package deliverable

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubUpstreamReader implements UpstreamDeliverableReader for tool tests.
type stubUpstreamReader struct {
	out         biz.UpstreamDeliverableContent
	err         error
	gotTeamID   string
	gotMaxChars int
	calls       int
}

func (s *stubUpstreamReader) ReadUpstreamDeliverable(_ context.Context, teamID string, maxChars int) (biz.UpstreamDeliverableContent, error) {
	s.calls++
	s.gotTeamID = teamID
	s.gotMaxChars = maxChars
	return s.out, s.err
}

func TestReadUpstreamDeliverableTool_Declaration(t *testing.T) {
	tl := NewReadUpstreamDeliverableTool(&stubUpstreamReader{}, loggateway.NewNoop())
	decl := tl.Declaration()
	if decl.Name != "read_upstream_deliverable" {
		t.Fatalf("name=%q want read_upstream_deliverable", decl.Name)
	}
	if decl.InputSchema == nil || decl.InputSchema.Type != "object" {
		t.Fatalf("input schema missing or not object: %#v", decl.InputSchema)
	}
	if len(decl.InputSchema.Required) != 1 || decl.InputSchema.Required[0] != "team_id" {
		t.Fatalf("team_id must be the only required input, got %v", decl.InputSchema.Required)
	}
	if decl.OutputSchema == nil {
		t.Fatal("output schema should be non-nil")
	}
	if !strings.Contains(decl.Description, "上游") {
		t.Fatalf("description should explain the upstream-deliverable purpose, got %q", decl.Description)
	}
}

func TestReadUpstreamDeliverableTool_Call_HappyPath(t *testing.T) {
	reader := &stubUpstreamReader{out: biz.UpstreamDeliverableContent{
		Content:   "完整交付物全文",
		SizeChars: 7,
		Truncated: false,
		TeamID:    "t-up",
		SessionID: "sess-t-up",
	}}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())

	res, err := tl.Call(context.Background(), []byte(`{"team_id":"t-up"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reader.gotTeamID != "t-up" {
		t.Fatalf("reader got team_id %q", reader.gotTeamID)
	}
	if reader.gotMaxChars != biz.DefaultUpstreamDeliverableMaxChars {
		t.Fatalf("unset max_chars should pass the default budget %d, got %d", biz.DefaultUpstreamDeliverableMaxChars, reader.gotMaxChars)
	}
	out, ok := res.(readUpstreamDeliverableOutput)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if out.Content != "完整交付物全文" || out.SizeChars != 7 || out.Truncated {
		t.Fatalf("output mapping mismatch: %+v", out)
	}
	if out.TeamID != "t-up" || out.SessionID != "sess-t-up" {
		t.Fatalf("ids mismatch: %+v", out)
	}
}

func TestReadUpstreamDeliverableTool_Call_MaxCharsCapped(t *testing.T) {
	reader := &stubUpstreamReader{}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())

	if _, err := tl.Call(context.Background(), []byte(`{"team_id":"t1","max_chars":999999}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reader.gotMaxChars != biz.MaxUpstreamDeliverableChars {
		t.Fatalf("max_chars above the cap should clamp to %d, got %d", biz.MaxUpstreamDeliverableChars, reader.gotMaxChars)
	}
}

func TestReadUpstreamDeliverableTool_Call_RequiresTeamID(t *testing.T) {
	reader := &stubUpstreamReader{}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())

	for _, args := range []string{`{}`, `{"team_id":"  "}`} {
		if _, err := tl.Call(context.Background(), []byte(args)); err == nil {
			t.Fatalf("args %s should error on missing team_id", args)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("reader must not be called on invalid input, got %d calls", reader.calls)
	}
}

func TestReadUpstreamDeliverableTool_Call_InvalidJSON(t *testing.T) {
	tl := NewReadUpstreamDeliverableTool(&stubUpstreamReader{}, loggateway.NewNoop())
	if _, err := tl.Call(context.Background(), []byte(`{bad`)); err == nil {
		t.Fatal("invalid JSON args should error")
	}
}

func TestReadUpstreamDeliverableTool_Call_ReaderErrorPropagates(t *testing.T) {
	reader := &stubUpstreamReader{err: errors.New("upstream team has not completed yet")}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())
	if _, err := tl.Call(context.Background(), []byte(`{"team_id":"t-run"}`)); err == nil {
		t.Fatal("reader error should propagate")
	}
}

// Nil reader must not panic (defensive wiring guard).
func TestReadUpstreamDeliverableTool_Call_NilReader(t *testing.T) {
	tl := NewReadUpstreamDeliverableTool(nil, loggateway.NewNoop())
	if _, err := tl.Call(context.Background(), []byte(`{"team_id":"t1"}`)); err == nil {
		t.Fatal("nil reader should error, not panic")
	}
}

// The output must stay JSON-marshalable (the runtime serializes tool results).
func TestReadUpstreamDeliverableTool_OutputJSONShape(t *testing.T) {
	reader := &stubUpstreamReader{out: biz.UpstreamDeliverableContent{
		Content: "x", SizeChars: 1, TeamID: "t", SessionID: "s",
	}}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())
	res, err := tl.Call(context.Background(), []byte(`{"team_id":"t"}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("output must marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("output must unmarshal to object: %v", err)
	}
	for _, key := range []string{"content", "size_chars", "truncated", "team_id", "session_id"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("output JSON missing key %q: %s", key, b)
		}
	}
}
