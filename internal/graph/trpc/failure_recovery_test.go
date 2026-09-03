package graph

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcgraph "trpc.group/trpc-go/trpc-agent-go/graph"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestFailureRecoveryAfterNode_skipOnFailure(t *testing.T) {
	cb := failureRecoveryAfterNode(NodeDef{
		NodeDef: biz.NodeDef{ID: "member-1", FailureAction: biz.FailureOnFailureSkip},
	}, nil)
	state := trpcgraph.State{}
	out, err := cb(context.Background(), &trpcgraph.NodeCallbackContext{NodeID: "member-1"}, state, nil, errors.New("boom"))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := out.(map[string]any)
	if !ok || m[biz.SkippedNodeOutputKey] != "member-1" {
		t.Fatalf("out=%v", out)
	}
	skipped, ok := state[biz.SkippedNodesStateKey].([]string)
	if !ok || len(skipped) != 1 || skipped[0] != "member-1" {
		t.Fatalf("state=%v", state[biz.SkippedNodesStateKey])
	}
	// P1-2：skip 必须向 messages 注入成员失败通告（下游合成者可见），
	// 且通告携带错误原因。
	msgs, ok := state[trpcgraph.StateKeyMessages].([]trpcmodel.Message)
	if !ok || len(msgs) != 1 {
		t.Fatalf("notice message missing, state messages=%v", state[trpcgraph.StateKeyMessages])
	}
	if !strings.Contains(msgs[0].Content, "member-1") || !strings.Contains(msgs[0].Content, "boom") {
		t.Fatalf("notice content = %q, want node id + error", msgs[0].Content)
	}
	if msgs[0].Role != trpcmodel.RoleAssistant {
		t.Fatalf("notice role = %q, want assistant", msgs[0].Role)
	}
}

// P1-2：通告追加到既有 messages 末尾（不覆盖成员已产出消息）；错误文本
// 超长按 skipNoticeErrMaxLen 截断。
func TestFailureRecoveryAfterNode_skipNoticeAppendsAndTruncates(t *testing.T) {
	cb := failureRecoveryAfterNode(NodeDef{
		NodeDef: biz.NodeDef{ID: "member-9", FailureAction: biz.FailureOnFailureSkip},
	}, nil)
	existing := trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: "worker output"}
	state := trpcgraph.State{trpcgraph.StateKeyMessages: []trpcmodel.Message{existing}}
	longErr := errors.New(strings.Repeat("x", skipNoticeErrMaxLen+50))
	if _, err := cb(context.Background(), &trpcgraph.NodeCallbackContext{NodeID: "member-9"}, state, nil, longErr); err != nil {
		t.Fatal(err)
	}
	msgs, ok := state[trpcgraph.StateKeyMessages].([]trpcmodel.Message)
	if !ok || len(msgs) != 2 {
		t.Fatalf("messages len = %v, want existing+notice", state[trpcgraph.StateKeyMessages])
	}
	if msgs[0].Content != "worker output" {
		t.Fatalf("existing message overwritten: %q", msgs[0].Content)
	}
	if len([]rune(msgs[1].Content)) > skipNoticeErrMaxLen+80 {
		t.Fatalf("notice not truncated: len=%d", len([]rune(msgs[1].Content)))
	}
}

func TestBuildStateGraph_skipOnFailureRecovery(t *testing.T) {
	failFunc := func(_ context.Context, _ trpcgraph.State) (any, error) {
		return nil, errors.New("boom")
	}
	reg := NewRegistry()
	reg.RegisterNodeFuncInstance("fail-func", failFunc)
	cfg := GraphBuildConfig{
		EntryPoint:  "fail",
		FinishPoint: "fail",
		StateFields: []StateFieldDef{{Name: biz.SkippedNodesStateKey, Type: "[]string", Reducer: ReducerAppend}},
		Nodes: []biz.NodeDef{{
			ID: "fail", Type: "function", FailureAction: biz.FailureOnFailureSkip, FuncRef: "fail-func",
		}},
	}
	g, _, err := BuildStateGraphWithRegistryAndLogger(context.Background(), cfg, reg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec, err := trpcgraph.NewExecutor(g)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := exec.Execute(context.Background(), trpcgraph.State{}, &trpcagent.Invocation{InvocationID: "inv-skip-recover"})
	if err != nil {
		t.Fatal(err)
	}
	var done bool
	for ev := range ch {
		if ev.Done {
			done = true
		}
	}
	if !done {
		t.Fatal("expected completed execution after skip recovery")
	}
}

type parentWithSubAgent struct{ a trpcagent.Agent }

func (p *parentWithSubAgent) FindSubAgent(name string) trpcagent.Agent {
	if p.a != nil && p.a.Info().Name == name {
		return p.a
	}
	return nil
}

type okSubAgent struct{ name string }

func (a *okSubAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	ch := make(chan *trpcevent.Event)
	close(ch)
	return ch, nil
}
func (a *okSubAgent) Tools() []trpctool.Tool                   { return nil }
func (a *okSubAgent) Info() trpcagent.Info                     { return trpcagent.Info{Name: a.name} }
func (a *okSubAgent) SubAgents() []trpcagent.Agent             { return nil }
func (a *okSubAgent) FindSubAgent(name string) trpcagent.Agent { return nil }

func TestFailureRecoveryAfterNode_fallbackAgent(t *testing.T) {
	backup := &okSubAgent{name: "backup-key"}
	exec := &trpcgraph.ExecutionContext{InvocationID: "inv-fallback", EventChan: make(chan *trpcevent.Event, 4)}
	state := trpcgraph.State{
		trpcgraph.StateKeyExecContext:   exec,
		trpcgraph.StateKeyCurrentNodeID: "member-1",
		trpcgraph.StateKeyParentAgent:   &parentWithSubAgent{a: backup},
	}
	cb := failureRecoveryAfterNode(NodeDef{
		NodeDef: biz.NodeDef{ID: "member-1", AgentName: "primary-key", FallbackAgent: "backup-key"},
	}, backup)
	out, err := cb(context.Background(), &trpcgraph.NodeCallbackContext{NodeID: "member-1"}, state, nil, errors.New("primary failed"))
	if err != nil {
		t.Fatal(err)
	}
	st, ok := out.(trpcgraph.State)
	if !ok {
		t.Fatalf("out type=%T", out)
	}
	if st["_fallback_from_member-1"] != "primary-key" || st["_fallback_agent_member-1"] != "backup-key" {
		t.Fatalf("fallback markers: %v", st)
	}
}
