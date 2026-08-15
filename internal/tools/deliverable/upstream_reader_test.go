package deliverable

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// stubUpstreamReader implements UpstreamDeliverableReader for tool tests.
type stubUpstreamReader struct {
	out                biz.UpstreamDeliverableContent
	err                error
	gotReaderSessionID string
	gotTeamID          string
	gotMaxChars        int
	calls              int
	keyOut             biz.UpstreamDeliverableContent
	keyErr             error
	gotKey             string
	keyCalls           int
}

func (s *stubUpstreamReader) ReadUpstreamDeliverable(_ context.Context, readerSessionID, teamID string, maxChars int) (biz.UpstreamDeliverableContent, error) {
	s.calls++
	s.gotReaderSessionID = readerSessionID
	s.gotTeamID = teamID
	s.gotMaxChars = maxChars
	return s.out, s.err
}

func (s *stubUpstreamReader) ReadUpstreamDeliverableKey(_ context.Context, readerSessionID, teamID, key string, maxChars int) (biz.UpstreamDeliverableContent, error) {
	s.keyCalls++
	s.gotReaderSessionID = readerSessionID
	s.gotTeamID = teamID
	s.gotKey = key
	s.gotMaxChars = maxChars
	return s.keyOut, s.keyErr
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

// The calling agent's session ID (from the invocation context) must be
// forwarded to the biz reader so it can run the runtime contract check
// against the reader team's InputContract (Phase B).
func TestReadUpstreamDeliverableTool_Call_PassesInvocationSessionID(t *testing.T) {
	reader := &stubUpstreamReader{}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())

	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: "  sess-reader-1  "}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)

	if _, err := tl.Call(ctx, []byte(`{"team_id":"t-up"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reader.gotReaderSessionID != "sess-reader-1" {
		t.Fatalf("reader session id should be forwarded (trimmed), got %q", reader.gotReaderSessionID)
	}
}

// Without an invocation context (CLI / direct calls), the reader session ID
// stays empty and the biz layer skips the contract check.
func TestReadUpstreamDeliverableTool_Call_NoInvocation_EmptyReaderSession(t *testing.T) {
	reader := &stubUpstreamReader{}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())

	if _, err := tl.Call(context.Background(), []byte(`{"team_id":"t-up"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reader.gotReaderSessionID != "" {
		t.Fatalf("no invocation → empty reader session id, got %q", reader.gotReaderSessionID)
	}
}

// key 参数路由：带 key 时必须走按 key 单取载荷（长文场景下游只取契约文章），
// 不触达全文读取路径；输出回显 key。
func TestReadUpstreamDeliverableTool_Call_KeyRoutesToKeyedRead(t *testing.T) {
	reader := &stubUpstreamReader{keyOut: biz.UpstreamDeliverableContent{
		Content:   "# 深度文章全文\n……",
		SizeChars: 9,
		TeamID:    "t-up",
		SessionID: "sess-t-up",
	}}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())

	res, err := tl.Call(context.Background(), []byte(`{"team_id":"t-up","key":" article "}`))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reader.keyCalls != 1 || reader.calls != 0 {
		t.Fatalf("keyed read expected exactly once (key=%d full=%d)", reader.keyCalls, reader.calls)
	}
	if reader.gotKey != "article" {
		t.Fatalf("key should be forwarded trimmed, got %q", reader.gotKey)
	}
	if reader.gotMaxChars != biz.DefaultUpstreamDeliverableMaxChars {
		t.Fatalf("unset max_chars should pass the default budget, got %d", reader.gotMaxChars)
	}
	out := res.(readUpstreamDeliverableOutput)
	if out.Key != "article" || out.Content != reader.keyOut.Content {
		t.Fatalf("output should echo key and keyed content: %+v", out)
	}
}

// 无 key 时不触达按 key 读取路径（全文路径回归保护）。
func TestReadUpstreamDeliverableTool_Call_NoKeySkipsKeyedRead(t *testing.T) {
	reader := &stubUpstreamReader{}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())
	if _, err := tl.Call(context.Background(), []byte(`{"team_id":"t-up"}`)); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if reader.keyCalls != 0 {
		t.Fatalf("keyed read must not run without key, got %d calls", reader.keyCalls)
	}
}

// 按 key 读取的 biz 错误（未知 key / 保留 key 拒绝）必须透传给调用方。
func TestReadUpstreamDeliverableTool_Call_KeyErrorPropagates(t *testing.T) {
	reader := &stubUpstreamReader{keyErr: errors.New(`key "nope" 不存在于交付物载荷`)}
	tl := NewReadUpstreamDeliverableTool(reader, loggateway.NewNoop())
	if _, err := tl.Call(context.Background(), []byte(`{"team_id":"t-up","key":"nope"}`)); err == nil {
		t.Fatal("keyed reader error should propagate")
	}
}
