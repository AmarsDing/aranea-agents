package team

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
)

// metadataKeyCall records an UpdateMetadataKey invocation for S6 assertions.
type metadataKeyCall struct {
	id, key, value string
}

// fakeSwarmSessions embeds the composite interface (nil for unused methods)
// and overrides the two write paths writeSwarmActiveAgent could take.
type fakeSwarmSessions struct {
	biz.SessionTurnManager
	keyCalls    []metadataKeyCall
	updateCalls int
}

func (f *fakeSwarmSessions) UpdateMetadataKey(_ context.Context, id, key, value string) error {
	f.keyCalls = append(f.keyCalls, metadataKeyCall{id: id, key: key, value: value})
	return nil
}

func (f *fakeSwarmSessions) Update(_ context.Context, _ string, _ session.SessionUpdateFields) (session.Session, error) {
	f.updateCalls++
	return session.Session{}, nil
}

// S6（swarm CAS）：writeSwarmActiveAgent 必须走单 key 原子更新
// （UpdateMetadataKey / jsonb_set），禁止全文档 Update——后者基于陈旧快照
// read-modify-write，并发 transfer 时会覆盖其他子系统写入的 metadata key。
func TestWriteSwarmActiveAgent_UsesAtomicKeyUpdate(t *testing.T) {
	fake := &fakeSwarmSessions{}
	sess := biz.Session{ID: "sess-1", MetadataJSON: `{"unrelated_key":"keep-me"}`}

	writeSwarmActiveAgent(context.Background(), fake, sess, "beta")

	if fake.updateCalls != 0 {
		t.Fatalf("full-document Update must not be used (lost-update race), got %d calls", fake.updateCalls)
	}
	if len(fake.keyCalls) != 1 {
		t.Fatalf("want 1 UpdateMetadataKey call, got %d", len(fake.keyCalls))
	}
	call := fake.keyCalls[0]
	if call.id != "sess-1" || call.key != biz.SwarmActiveAgentSessionMeta || call.value != "beta" {
		t.Fatalf("unexpected UpdateMetadataKey args: %+v", call)
	}
}

// 空 agentKey / nil sessions 不得触发任何写。
func TestWriteSwarmActiveAgent_Guards(t *testing.T) {
	fake := &fakeSwarmSessions{}
	writeSwarmActiveAgent(context.Background(), fake, biz.Session{ID: "s"}, "  ")
	writeSwarmActiveAgent(context.Background(), nil, biz.Session{ID: "s"}, "beta")
	if fake.updateCalls != 0 || len(fake.keyCalls) != 0 {
		t.Fatalf("guards violated: updateCalls=%d keyCalls=%v", fake.updateCalls, fake.keyCalls)
	}
}

func TestApplyCrossRequestEntryOverride(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		EntryPoint: "member-1",
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: biz.NodeTypeAgent, AgentName: "alpha"},
			{ID: "member-2", Type: biz.NodeTypeAgent, AgentName: "beta"},
		},
	}
	def := Definition{Swarm: &SwarmConfigDef{CrossRequestTransfer: true}}

	got := applyCrossRequestEntryOverride(cfg, def, "beta")
	if got.EntryPoint != "member-2" {
		t.Fatalf("want member-2, got %s", got.EntryPoint)
	}

	def.Swarm.CrossRequestTransfer = false
	got = applyCrossRequestEntryOverride(cfg, def, "beta")
	if got.EntryPoint != "member-1" {
		t.Fatalf("disabled should keep entry, got %s", got.EntryPoint)
	}
}

func TestReadSwarmActiveAgent(t *testing.T) {
	sess := biz.Session{MetadataJSON: `{"swarm_active_agent":"beta","other":1}`}
	if got := readSwarmActiveAgent(sess); got != "beta" {
		t.Fatalf("want beta, got %q", got)
	}
	if got := readSwarmActiveAgent(biz.Session{}); got != "" {
		t.Fatalf("empty meta => empty, got %q", got)
	}
}

func TestApplySwarmGraphConfig(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: biz.NodeTypeAgent},
			{ID: "tool-1", Type: biz.NodeTypeTool},
		},
	}
	def := Definition{Swarm: &SwarmConfigDef{
		MaxHandoffs:                5,
		RepetitiveHandoffWindow:    4,
		RepetitiveHandoffMinUnique: 2,
		NodeTimeoutSeconds:         30,
		CrossRequestTransfer:       true,
	}}
	got := applySwarmGraphConfig(cfg, def)
	if got.SwarmSafety == nil || got.SwarmSafety.MaxHandoffs != 5 {
		t.Fatalf("SwarmSafety not set: %+v", got.SwarmSafety)
	}
	if got.Nodes[0].TimeoutSeconds != 30 || !got.Nodes[0].IsolatedMessages {
		t.Fatalf("agent node not updated: %+v", got.Nodes[0])
	}
	if got.Nodes[1].TimeoutSeconds != 30 {
		t.Fatalf("tool timeout not set: %+v", got.Nodes[1])
	}
}
