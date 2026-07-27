package tools

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcartifact "trpc.group/trpc-go/trpc-agent-go/artifact"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// mockArtifactService is an in-memory artifact.Service double for offload tests.
type mockArtifactService struct {
	saved     map[string][]byte
	versions  map[string]int
	saveCalls int
}

func newMockArtifactService() *mockArtifactService {
	return &mockArtifactService{
		saved:    make(map[string][]byte),
		versions: make(map[string]int),
	}
}

func (m *mockArtifactService) SaveArtifact(
	_ context.Context,
	_ trpcartifact.SessionInfo,
	filename string,
	a *trpcartifact.Artifact,
) (int, error) {
	ver := m.versions[filename]
	m.versions[filename] = ver + 1
	m.saved[filename] = a.Data
	m.saveCalls++
	return ver, nil
}

func (m *mockArtifactService) LoadArtifact(
	_ context.Context,
	_ trpcartifact.SessionInfo,
	filename string,
	_ *int,
) (*trpcartifact.Artifact, error) {
	data, ok := m.saved[filename]
	if !ok {
		return nil, nil
	}
	return &trpcartifact.Artifact{Data: data, Name: filename}, nil
}

func (m *mockArtifactService) ListArtifactKeys(
	_ context.Context,
	_ trpcartifact.SessionInfo,
) ([]string, error) {
	return nil, nil
}

func (m *mockArtifactService) DeleteArtifact(
	_ context.Context,
	_ trpcartifact.SessionInfo,
	_ string,
) error {
	return nil
}

func (m *mockArtifactService) ListVersions(
	_ context.Context,
	_ trpcartifact.SessionInfo,
	_ string,
) ([]int, error) {
	return nil, nil
}

// invocationCtxWithArtifact builds an invocation context carrying both a
// session and an artifact service, satisfying the offload prerequisites.
func invocationCtxWithArtifact(appName, userID, sessionID string, svc trpcartifact.Service) context.Context {
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{
			AppName: appName,
			UserID:  userID,
			ID:      sessionID,
		},
		ArtifactService: svc,
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// TestToolDecorator_OffloadsLargeResultToArtifact verifies that an oversized
// result is persisted via the artifact service and replaced with an
// offload envelope carrying a resolvable ref plus dual-end previews,
// instead of being irreversibly truncated.
func TestToolDecorator_OffloadsLargeResultToArtifact(t *testing.T) {
	big := strings.Repeat("x", 30*1024)
	svc := newMockArtifactService()
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return map[string]any{"data": big}, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	ctx := invocationCtxWithArtifact("agent-a", "user-1", "sess-1", svc)
	res, err := d.Call(ctx, []byte(`{"q":"aranea"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want map[string]any envelope", res)
	}
	if offloaded, _ := env["offloaded"].(bool); !offloaded {
		t.Fatalf("expected offloaded=true envelope, got %v", env)
	}
	if truncated, _ := env["truncated"].(bool); truncated {
		t.Fatalf("offload path must not produce a truncation envelope: %v", env)
	}
	ref, _ := env["ref"].(string)
	if !strings.HasPrefix(ref, "artifact://tool_results/web_search/") {
		t.Fatalf("ref = %q, want artifact://tool_results/web_search/ prefix", ref)
	}
	if svc.saveCalls != 1 {
		t.Fatalf("SaveArtifact calls = %d, want 1", svc.saveCalls)
	}
	// The persisted payload must be the complete original JSON (no loss).
	var saved []byte
	for _, data := range svc.saved {
		saved = data
	}
	if !strings.Contains(string(saved), big) {
		t.Fatalf("saved artifact lost data: len(saved)=%d, want original 30KB payload", len(saved))
	}
	// Dual-end previews: head present, tail present, hint guides read-back.
	head, _ := env["preview_head"].(string)
	tail, _ := env["preview_tail"].(string)
	if head == "" || tail == "" {
		t.Fatalf("preview_head/preview_tail must be non-empty: %v", env)
	}
	if hint, _ := env["read_hint"].(string); !strings.Contains(hint, "read_file") {
		t.Fatalf("read_hint must point to read_file paging, got %q", hint)
	}
	// The ref must resolve back to the full payload via the artifact service.
	art, err := svc.LoadArtifact(ctx, trpcartifact.SessionInfo{}, ref[strings.Index(ref, "://")+3:strings.LastIndex(ref, "@")], nil)
	if err != nil || art == nil {
		t.Fatalf("ref %q did not resolve: art=%v err=%v", ref, art, err)
	}
}

// TestToolDecorator_OffloadDeterministicRef verifies the same tool + args
// produce the same artifact filename (idempotent naming, no storage bloat).
func TestToolDecorator_OffloadDeterministicRef(t *testing.T) {
	big := strings.Repeat("y", 20*1024)
	svc := newMockArtifactService()
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return big, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	ctx := invocationCtxWithArtifact("agent-a", "user-1", "sess-1", svc)
	args := []byte(`{"q":"same"}`)
	r1, _ := d.Call(ctx, args)
	r2, _ := d.Call(ctx, args)
	ref1 := r1.(map[string]any)["ref"].(string)
	ref2 := r2.(map[string]any)["ref"].(string)
	// Same artifact filename; only the version suffix may differ.
	base1 := ref1[:strings.LastIndex(ref1, "@")]
	base2 := ref2[:strings.LastIndex(ref2, "@")]
	if base1 != base2 {
		t.Fatalf("same args produced different artifact files: %q vs %q", base1, base2)
	}
	if len(svc.saved) != 1 {
		t.Fatalf("expected 1 unique artifact file, got %d", len(svc.saved))
	}
}

// TestToolDecorator_ReadFileExcludedFromOffload verifies read_file results
// never offload (reading back an offloaded read_file result would require
// read_file again — a circular regress); they fall back to truncation.
func TestToolDecorator_ReadFileExcludedFromOffload(t *testing.T) {
	big := strings.Repeat("z", 20*1024)
	svc := newMockArtifactService()
	tool := &decoratorMockTool{
		name: "read_file",
		call: func(_ context.Context, _ []byte) (any, error) {
			return big, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	ctx := invocationCtxWithArtifact("agent-a", "user-1", "sess-1", svc)
	res, err := d.Call(ctx, []byte(`{"file_name":"big.log"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want truncation envelope", res)
	}
	if truncated, _ := env["truncated"].(bool); !truncated {
		t.Fatalf("read_file must fall back to truncation envelope, got %v", env)
	}
	if svc.saveCalls != 0 {
		t.Fatalf("read_file must not offload: SaveArtifact calls = %d", svc.saveCalls)
	}
}

// TestToolDecorator_OffloadFallsBackWithoutArtifactService verifies that
// when the invocation cannot persist artifacts (no service configured),
// behavior is byte-identical to the legacy truncation envelope.
func TestToolDecorator_OffloadFallsBackWithoutArtifactService(t *testing.T) {
	big := strings.Repeat("w", 20*1024)
	tool := &decoratorMockTool{
		name: "web_search",
		call: func(_ context.Context, _ []byte) (any, error) {
			return big, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger:       loggateway.NewNoop(),
		ResultBudget: &ResultBudget{MaxBytes: 1024, Mode: "tail"},
	})
	// Invocation present but ArtifactService nil.
	ctx := invocationCtx("agent-a", "user-1", "sess-1")
	res, err := d.Call(ctx, []byte(`{}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	env, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want truncation envelope", res)
	}
	if truncated, _ := env["truncated"].(bool); !truncated {
		t.Fatalf("expected legacy truncation envelope without artifact service, got %v", env)
	}
}
