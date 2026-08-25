package agent

// tool_result_prune_inject_test.go — R2 确定性剪枝 hook 单测
// （79-runtime-governance 开发计划 1.3d）：阈值 / 轮距 / pair 完整 / 豁免 /
// 归档幂等 / 计数 / fail-soft / kill switch。

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// ---------------------------------------------------------------------------
// fakes
// ---------------------------------------------------------------------------

type fakePruneBlobStore struct {
	blobs    map[string]*biz.ToolResultBlob
	failSave bool
}

func newFakePruneBlobStore() *fakePruneBlobStore {
	return &fakePruneBlobStore{blobs: map[string]*biz.ToolResultBlob{}}
}

func (f *fakePruneBlobStore) GetBlob(_ context.Context, id string) (*biz.ToolResultBlob, error) {
	return f.blobs[id], nil
}

func (f *fakePruneBlobStore) ListBlobsBySession(_ context.Context, _ string, _ int) ([]*biz.ToolResultBlob, error) {
	return nil, nil
}

func (f *fakePruneBlobStore) SaveBlob(_ context.Context, blob *biz.ToolResultBlob) error {
	if f.failSave {
		return errors.New("db down")
	}
	f.blobs[blob.ID] = blob
	return nil
}

type fakePruneReplacementStore struct {
	byMessage map[string]*biz.ToolResultReplacement
}

func newFakePruneReplacementStore() *fakePruneReplacementStore {
	return &fakePruneReplacementStore{byMessage: map[string]*biz.ToolResultReplacement{}}
}

func pruneRepKey(sessionID, messageID string) string { return sessionID + "\x00" + messageID }

func (f *fakePruneReplacementStore) GetReplacementByMessage(_ context.Context, sessionID, messageID string) (*biz.ToolResultReplacement, error) {
	return f.byMessage[pruneRepKey(sessionID, messageID)], nil
}

func (f *fakePruneReplacementStore) SaveReplacement(_ context.Context, r *biz.ToolResultReplacement) error {
	f.byMessage[pruneRepKey(r.SessionID, r.MessageID)] = r
	return nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

const pruneTestBigContent = 5000 // bytes，> 默认 S=4096

func pruneTestGate(blobs *fakePruneBlobStore, reps *fakePruneReplacementStore) *biz.ToolResultGate {
	return biz.NewToolResultGate(blobs, blobs, reps, reps)
}

func pruneTestCtx() context.Context {
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: "sess-prune"}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// pruneTestMessages 构造 3 轮会话：tool 结果位于第 1 轮（idx 3），其后真实
// user 消息 = t2/t3 共 2 条（轮距 2）。assistant 消息带 ToolCalls 以验证
// pair 完整（call 侧消息零改动）。
func pruneTestMessages(toolContent string) []trpcmodel.Message {
	return []trpcmodel.Message{
		trpcmodel.NewSystemMessage("sys"),
		trpcmodel.NewUserMessage("t1"),
		{Role: trpcmodel.RoleAssistant, Content: "calling tool", ToolCalls: []trpcmodel.ToolCall{{ID: "call-1"}}},
		{Role: trpcmodel.RoleTool, ToolName: "big_tool", ToolID: "call-1", Content: toolContent},
		trpcmodel.NewUserMessage("t2"),
		trpcmodel.NewAssistantMessage("a2"),
		trpcmodel.NewUserMessage("t3"),
	}
}

func runPruneHook(t *testing.T, hook callbacks.Callback, ctx context.Context, msgs []trpcmodel.Message) []trpcmodel.Message {
	t.Helper()
	h, ok := hook.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatal("prune hook must implement BeforeModelHook")
	}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	if _, err := h.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("prune hook failed: %v", err)
	}
	return args.Request.Messages
}

func defaultPruneCfg() ToolResultPruneConfig {
	return ToolResultPruneConfig{Enabled: true, AfterTurns: 1, SizeBytes: 4096}
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// 主路径：轮距>K 且超阈 → 剪枝；指针含 blob id/工具名；pair 完整；计数正确。
func TestToolResultPrune_PrunesOldOversizedResult(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), defaultPruneCfg(), loggateway.NewNoop())
	if hook == nil {
		t.Fatal("hook must register when gate non-nil and enabled")
	}
	ctx := pruneTestCtx()
	big := strings.Repeat("x", pruneTestBigContent)
	msgs := pruneTestMessages(big)

	out := runPruneHook(t, hook, ctx, msgs)

	// pair 完整：消息条数/角色序不变；call 侧 assistant 消息字节不动。
	if len(out) != len(msgs) {
		t.Fatalf("message count changed: %d → %d", len(msgs), len(out))
	}
	assistant := out[2]
	if assistant.Role != trpcmodel.RoleAssistant || assistant.Content != "calling tool" || len(assistant.ToolCalls) != 1 {
		t.Fatalf("pair violated — tool_call side message mutated: %+v", assistant)
	}

	pruned := out[3]
	if pruned.Role != trpcmodel.RoleTool || pruned.ToolID != "call-1" || pruned.ToolName != "big_tool" {
		t.Fatalf("pruned message identity fields must survive: %+v", pruned)
	}
	if pruned.Content == big {
		t.Fatal("content must be replaced by prune pointer")
	}
	if len(pruned.ContentParts) != 0 {
		t.Fatal("ContentParts must be cleared after prune")
	}

	// 归档：恰好 1 blob + 1 replacement，blob 存原文，指针引用 blob id。
	if len(blobs.blobs) != 1 {
		t.Fatalf("blob count = %d, want 1", len(blobs.blobs))
	}
	var blob *biz.ToolResultBlob
	for _, b := range blobs.blobs {
		blob = b
	}
	if blob.FullContent != big || blob.ToolName != "big_tool" || blob.SessionID != "sess-prune" {
		t.Fatalf("blob must persist original content: %+v", blob)
	}
	rep := reps.byMessage[pruneRepKey("sess-prune", "call-1")]
	if rep == nil || rep.ResultBlobID != blob.ID {
		t.Fatalf("replacement must pin blob id: %+v", rep)
	}
	wantPointer := biz.ToolResultPrunePointer(pruneTestBigContent, "big_tool", blob.ID)
	if pruned.Content != wantPointer {
		t.Fatalf("pointer mismatch:\n got %q\nwant %q", pruned.Content, wantPointer)
	}

	// 计数：prune_count=1，prune_bytes=原文字节数。
	meta := LoadPruneMeta(ctx)
	if meta.Count != 1 || meta.Bytes != pruneTestBigContent {
		t.Fatalf("prune meta = %+v, want {1 %d}", meta, pruneTestBigContent)
	}
}

// 轮距豁免：K=2 时轮距=2 不剪（≤K 豁免）。
func TestToolResultPrune_WithinKTurnsExempt(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	cfg := defaultPruneCfg()
	cfg.AfterTurns = 2
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), cfg, loggateway.NewNoop())
	big := strings.Repeat("x", pruneTestBigContent)
	out := runPruneHook(t, hook, pruneTestCtx(), pruneTestMessages(big))
	if out[3].Content != big {
		t.Fatal("result within K turns must not be pruned")
	}
	if len(blobs.blobs) != 0 {
		t.Fatal("no archive expected for exempt result")
	}
}

// 尺寸豁免：未超 S 不剪。
func TestToolResultPrune_BelowSizeThresholdExempt(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), defaultPruneCfg(), loggateway.NewNoop())
	small := strings.Repeat("y", 1000)
	out := runPruneHook(t, hook, pruneTestCtx(), pruneTestMessages(small))
	if out[3].Content != small {
		t.Fatal("below-threshold result must not be pruned")
	}
}

// 白名单豁免：exempt_tools 命中不剪。
func TestToolResultPrune_ExemptToolsWhitelist(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	cfg := defaultPruneCfg()
	cfg.ExemptTools = map[string]bool{"big_tool": true}
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), cfg, loggateway.NewNoop())
	big := strings.Repeat("x", pruneTestBigContent)
	out := runPruneHook(t, hook, pruneTestCtx(), pruneTestMessages(big))
	if out[3].Content != big {
		t.Fatal("whitelisted tool result must not be pruned")
	}
}

// 错误结果豁免："Error:" 前缀为失败重试证据，不剪。
func TestToolResultPrune_ErrorResultExempt(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), defaultPruneCfg(), loggateway.NewNoop())
	errContent := "Error: connection timeout\n" + strings.Repeat("stack ", 1000)
	out := runPruneHook(t, hook, pruneTestCtx(), pruneTestMessages(errContent))
	if out[3].Content != errContent {
		t.Fatal("error result must not be pruned")
	}
}

// 轮距口径：尾部 dynamic cue 是 user-role 哨兵，不得计入轮距——K=2 时
// 追加 cue 后轮距仍=2（豁免），若误计为 3 将错误剪枝。
func TestToolResultPrune_DynamicCueNotCountedAsTurn(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	cfg := defaultPruneCfg()
	cfg.AfterTurns = 2
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), cfg, loggateway.NewNoop())
	big := strings.Repeat("x", pruneTestBigContent)
	msgs := appendDynamicCue(pruneTestMessages(big), "intent-cue")
	out := runPruneHook(t, hook, pruneTestCtx(), msgs)
	if out[3].Content != big {
		t.Fatal("dynamic cue must not inflate turn distance")
	}
}

// 跨轮幂等：同一原文每轮重扫（会话存储不动），replacement 钉住 blob id，
// 第二轮归档返回既有 blob——blob 行不翻倍，指针字节跨轮稳定。
func TestToolResultPrune_IdempotentAcrossTurns(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	gate := pruneTestGate(blobs, reps)
	hook := newToolResultPruneBeforeHook(gate, defaultPruneCfg(), loggateway.NewNoop())
	big := strings.Repeat("x", pruneTestBigContent)

	out1 := runPruneHook(t, hook, pruneTestCtx(), pruneTestMessages(big))
	// 第二轮：会话存储原文不动 → 请求副本再次携带全量原文。
	out2 := runPruneHook(t, hook, pruneTestCtx(), pruneTestMessages(big))

	if len(blobs.blobs) != 1 {
		t.Fatalf("re-archive must reuse existing blob, got %d blobs", len(blobs.blobs))
	}
	if out1[3].Content != out2[3].Content {
		t.Fatalf("pointer must be byte-stable across turns:\n turn1 %q\n turn2 %q", out1[3].Content, out2[3].Content)
	}
}

// fail-soft：归档失败保留原文，本轮不省但不丢内容。
func TestToolResultPrune_ArchiveFailureKeepsOriginal(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	blobs.failSave = true
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), defaultPruneCfg(), loggateway.NewNoop())
	big := strings.Repeat("x", pruneTestBigContent)
	ctx := pruneTestCtx()
	out := runPruneHook(t, hook, ctx, pruneTestMessages(big))
	if out[3].Content != big {
		t.Fatal("archive failure must keep original content")
	}
	if meta := LoadPruneMeta(ctx); meta.Count != 0 {
		t.Fatalf("failed prune must not count, got %+v", meta)
	}
}

// kill switch / 依赖缺失：Enabled=false 或 gate=nil → hook 为 nil（零开销回退）。
func TestToolResultPrune_KillSwitchAndNilGate(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	gate := pruneTestGate(blobs, reps)
	if hook := newToolResultPruneBeforeHook(nil, defaultPruneCfg(), loggateway.NewNoop()); hook != nil {
		t.Fatal("nil gate must skip registration")
	}
	cfg := defaultPruneCfg()
	cfg.Enabled = false
	if hook := newToolResultPruneBeforeHook(gate, cfg, loggateway.NewNoop()); hook != nil {
		t.Fatal("enabled=false must skip registration (kill switch)")
	}
}

// 无 invocation 上下文：sessionID 为空 → 零改写零计数（防御路径）。
func TestToolResultPrune_NoInvocationContext(t *testing.T) {
	blobs, reps := newFakePruneBlobStore(), newFakePruneReplacementStore()
	hook := newToolResultPruneBeforeHook(pruneTestGate(blobs, reps), defaultPruneCfg(), loggateway.NewNoop())
	big := strings.Repeat("x", pruneTestBigContent)
	out := runPruneHook(t, hook, context.Background(), pruneTestMessages(big))
	if out[3].Content != big {
		t.Fatal("no session id must skip pruning")
	}
	if meta := LoadPruneMeta(context.Background()); meta.Count != 0 {
		t.Fatalf("no invocation must yield zero meta, got %+v", meta)
	}
}
