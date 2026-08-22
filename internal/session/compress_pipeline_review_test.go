package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/compress"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- stubs ---

type stubLLMCompressor struct {
	calls   []compress.Request
	failErr map[int]error // 1-based call index → error to return
}

func (s *stubLLMCompressor) Compress(_ context.Context, req compress.Request) (compress.Result, error) {
	s.calls = append(s.calls, req)
	if s.failErr != nil {
		if err, ok := s.failErr[len(s.calls)]; ok {
			return compress.Result{}, err
		}
	}
	// ≥200 runes to stay clear of the degenerate-summary guard.
	md := fmt.Sprintf("SUMMARY-CHUNK-%d\n\n%s", len(s.calls), strings.Repeat("已确认的事实与结论。", 20))
	return compress.Result{Markdown: md, Provider: req.Provider, Model: req.Model, PromptVersion: compress.PromptVersion}, nil
}

func newLLMTestCompressor(read *stubCompressReadDeps, llm compress.Compressor) *Compressor {
	c := newTestCompressor(read)
	c.Compress = llm
	return c
}

// --- F2: chunked rolling summarization ---

// splitMessagesForCompress is the pure partitioning helper.
func TestSplitMessagesForCompress(t *testing.T) {
	msgs := make([]biz.ChatMessage, 0, 6)
	for i := 1; i <= 6; i++ {
		msgs = append(msgs, biz.ChatMessage{Role: "user", TurnNumber: i, ContentMarkdown: strings.Repeat("x", 100)})
	}
	chunks := splitMessagesForCompress(msgs, 250) // each msg renders ~110 runes → 2 per chunk
	if len(chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(chunks))
	}
	for i, ch := range chunks {
		if len(ch) != 2 {
			t.Fatalf("chunk %d len = %d, want 2", i, len(ch))
		}
	}
	// Order preserved: chunk 0 holds turns 1-2, chunk 2 holds turns 5-6.
	if chunks[0][0].TurnNumber != 1 || chunks[2][1].TurnNumber != 6 {
		t.Fatal("chunk ordering broken")
	}

	// Single message larger than maxRunes → its own chunk.
	big := []biz.ChatMessage{
		{Role: "user", TurnNumber: 1, ContentMarkdown: strings.Repeat("y", 1000)},
		{Role: "assistant", TurnNumber: 1, ContentMarkdown: "ok"},
	}
	chunks = splitMessagesForCompress(big, 250)
	if len(chunks) != 2 {
		t.Fatalf("oversize split chunks = %d, want 2", len(chunks))
	}
	if len(chunks[0]) != 1 || len(chunks[1]) != 1 {
		t.Fatal("oversize message must form its own chunk")
	}
}

// An oversized transcript must be summarized chunk-by-chunk, threading each
// chunk's output summary as PriorSummary into the next call (recursive absorb),
// instead of sending one giant transcript that overflows the compress model
// (context-overflow → deterministic failure → sticky suppression deadlock).
func TestLLMSummarize_ChunkedRollingAbsorb(t *testing.T) {
	defer func(prev int) { compressChunkMaxRunes = prev }(compressChunkMaxRunes)
	compressChunkMaxRunes = 600 // force chunking: each msg ~200 runes → ~2 msgs/chunk

	var msgs []biz.ChatMessage
	for i := 1; i <= 8; i++ {
		msgs = append(msgs,
			biz.ChatMessage{Role: "user", TurnNumber: i, ContentMarkdown: strings.Repeat("问", 80)},
			biz.ChatMessage{Role: "assistant", TurnNumber: i, ContentMarkdown: strings.Repeat("答", 80)},
		)
	}
	read := &stubCompressReadDeps{}
	llm := &stubLLMCompressor{}
	c := newLLMTestCompressor(read, llm)

	outcome := c.llmSummarize(context.Background(), biz.Session{}, biz.Agent{}, msgs, "summary", "sess-1")
	if outcome.level != compressLevelLLM || outcome.markdown == "" {
		t.Fatalf("outcome = %+v, want llm markdown", outcome)
	}
	if len(llm.calls) < 2 {
		t.Fatalf("expected chunked calls, got %d", len(llm.calls))
	}
	for i, req := range llm.calls {
		if i == 0 {
			if req.PriorSummary != "" {
				t.Fatalf("call 1 PriorSummary = %q, want empty (no prior rows)", req.PriorSummary)
			}
			continue
		}
		if !strings.HasPrefix(req.PriorSummary, fmt.Sprintf("SUMMARY-CHUNK-%d", i)) {
			t.Fatalf("call %d PriorSummary must be previous chunk output, got %q", i+1, req.PriorSummary[:min(40, len(req.PriorSummary))])
		}
	}
	// Final markdown is the last chunk's output (which recursively absorbed all priors).
	want := fmt.Sprintf("SUMMARY-CHUNK-%d", len(llm.calls))
	if !strings.HasPrefix(outcome.markdown, want) {
		t.Fatalf("final markdown must come from last chunk, want prefix %q", want)
	}
}

// A deterministic failure (e.g. context overflow) on any chunk aborts the
// whole cascade as deterministic — never retried, surfaced for suppression.
func TestLLMSummarize_ChunkDeterministicFailureAborts(t *testing.T) {
	defer func(prev int) { compressChunkMaxRunes = prev }(compressChunkMaxRunes)
	compressChunkMaxRunes = 600

	var msgs []biz.ChatMessage
	for i := 1; i <= 8; i++ {
		msgs = append(msgs,
			biz.ChatMessage{Role: "user", TurnNumber: i, ContentMarkdown: strings.Repeat("问", 80)},
			biz.ChatMessage{Role: "assistant", TurnNumber: i, ContentMarkdown: strings.Repeat("答", 80)},
		)
	}
	read := &stubCompressReadDeps{}
	llm := &stubLLMCompressor{failErr: map[int]error{
		2: &trpcmodel.ResponseError{Message: "context length exceeded"},
	}}
	c := newLLMTestCompressor(read, llm)

	outcome := c.llmSummarize(context.Background(), biz.Session{}, biz.Agent{}, msgs, "summary", "sess-1")
	if outcome.fail != compressFailureDeterministic {
		t.Fatalf("fail = %v, want deterministic", outcome.fail)
	}
	if len(llm.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (abort at failing chunk, no further chunks)", len(llm.calls))
	}
}

// --- F3: tool messages in compress transcript ---

func TestBuildCompressTranscript_RendersToolMessages(t *testing.T) {
	msgs := []biz.ChatMessage{
		{Role: "user", TurnNumber: 1, ContentMarkdown: "查一下"},
		{Role: "tool", TurnNumber: 1, ContentMarkdown: "result-body", OptionsJSON: `{"tool_name":"search"}`},
		{Role: "assistant", TurnNumber: 1, ContentMarkdown: "结果如下"},
	}
	got := buildCompressTranscript(msgs)
	if !strings.Contains(got, "TOOL(search): result-body") {
		t.Fatalf("transcript missing tool render: %q", got)
	}
}

func TestBuildCompressTranscript_TruncatesToolResult(t *testing.T) {
	long := strings.Repeat("r", 5000)
	msgs := []biz.ChatMessage{
		{Role: "tool", TurnNumber: 1, ContentMarkdown: long, OptionsJSON: `{"tool_name":"shell"}`},
	}
	got := buildCompressTranscript(msgs)
	if strings.Contains(got, long) {
		t.Fatal("tool result must be truncated")
	}
	if !strings.Contains(got, "truncated") {
		t.Fatal("truncation marker missing")
	}
	if len(got) > 1200 {
		t.Fatalf("truncated tool render too large: %d runes", len(got))
	}
}

func TestBuildCompressTranscript_TruncatesOversizeUserMessage(t *testing.T) {
	long := strings.Repeat("u", 20000)
	msgs := []biz.ChatMessage{{Role: "user", TurnNumber: 1, ContentMarkdown: long}}
	got := buildCompressTranscript(msgs)
	if strings.Contains(got, long) {
		t.Fatal("oversize user message must be truncated (single message can overflow a chunk otherwise)")
	}
	if !strings.Contains(got, "truncated") {
		t.Fatal("truncation marker missing")
	}
}

// loadCompressBody must surface tool messages inside the compressed turn range
// so the summarizer sees what was actually executed (design doc §6.4).
func TestLoadCompressBody_returnsToolBody(t *testing.T) {
	msgs := makeTimeline(5)
	// Tool message in body range (turn 1) and in tail range (turn 4).
	msgs = append(msgs,
		biz.ChatMessage{ID: "tool-body", Role: "tool", TurnNumber: 1, ContentMarkdown: "r1", OptionsJSON: `{"tool_name":"search"}`, CreatedAt: "2026-07-20T10:00:01Z"},
		biz.ChatMessage{ID: "tool-tail", Role: "tool", TurnNumber: 4, ContentMarkdown: "r4", OptionsJSON: `{"tool_name":"shell"}`, CreatedAt: "2026-07-20T10:00:04Z"},
	)
	read := &stubCompressReadDeps{maxSummarized: 0, msgs: msgs}
	c := newTestCompressor(read)

	_, _, toolBody, cutoffTurn, err := c.loadCompressBody(context.Background(), biz.Session{}, biz.Agent{}, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if cutoffTurn != 1 {
		t.Fatalf("cutoffTurn = %d, want 1", cutoffTurn)
	}
	if len(toolBody) != 1 || toolBody[0].ID != "tool-body" {
		t.Fatalf("toolBody = %+v, want only the turn-1 tool", toolBody)
	}
}

// hybrid / drop_tool_results strategies must NOT feed tool results to the LLM.
func TestLLMCompress_StrategyToolVisibility(t *testing.T) {
	mkBody := func() []biz.ChatMessage {
		var out []biz.ChatMessage
		for i := 1; i <= 3; i++ {
			out = append(out,
				biz.ChatMessage{Role: "user", TurnNumber: i, ContentMarkdown: strings.Repeat("问", 300)},
				biz.ChatMessage{Role: "assistant", TurnNumber: i, ContentMarkdown: strings.Repeat("答", 300)},
			)
		}
		return out
	}
	tools := []biz.ChatMessage{
		{Role: "tool", TurnNumber: 1, ContentMarkdown: "SECRET-TOOL-OUTPUT", OptionsJSON: `{"tool_name":"shell"}`},
	}

	run := func(strategy string) string {
		read := &stubCompressReadDeps{}
		llm := &stubLLMCompressor{}
		c := newLLMTestCompressor(read, llm)
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: strategy}}
		outcome := c.llmCompress(context.Background(), biz.Session{}, ag, mkBody(), tools, "sess-1")
		if outcome.markdown == "" {
			t.Fatalf("strategy %s: empty markdown", strategy)
		}
		if len(llm.calls) == 0 {
			t.Fatalf("strategy %s: no llm call", strategy)
		}
		return llm.calls[0].Transcript
	}

	if got := run("summary"); !strings.Contains(got, "TOOL(shell)") {
		t.Fatal("summary strategy must include tool messages in transcript")
	}
	if got := run("hybrid"); strings.Contains(got, "SECRET-TOOL-OUTPUT") {
		t.Fatal("hybrid strategy must exclude tool results from transcript")
	}
	if got := run("drop_tool_results"); strings.Contains(got, "SECRET-TOOL-OUTPUT") {
		t.Fatal("drop_tool_results strategy must exclude tool results from transcript")
	}
}

// --- F4: reserved-system-aware post-compression estimate ---

// Pre-compression ContextUsedTokens is provider-reported prompt_tokens (system
// prompt + tool schemas + content). The post-compression estimate must carry
// the same semantics, otherwise the trigger logic runs on a deflated value
// until the next authoritative update.
func TestEstimateCompactedPromptTokens_IncludesReservedSystem(t *testing.T) {
	tail := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: strings.Repeat("x", 250)},
	}
	contentOnly := roughTokenEstimate("summary\n" + strings.Repeat("x", 250) + "\n")
	got := estimateCompactedPromptTokens("summary", tail, 8000)
	if got < 8000 {
		t.Fatalf("est = %d, must include reservedSystem 8000", got)
	}
	want := 8000 + contentOnly
	if got != want {
		t.Fatalf("est = %d, want %d (reserved + content estimate)", got, want)
	}
}

// --- F6: ToolsProfile must drive reserved-system tokens ---

// Bug: softTriggerTokensPolicy & friends passed p.Model.SnapshotMode ("off"/"")
// as the profile argument, so reserved tokens always fell back to the default
// 8000 regardless of the agent's ToolsProfile.
func TestCompressPolicy_ReservedSystemUsesToolsProfile(t *testing.T) {
	const window = 100000
	coding := DefaultCompressPolicy()
	coding.Profile.ToolsProfile = "coding"
	chat := DefaultCompressPolicy()
	chat.Profile.ToolsProfile = "chat_only"

	softCoding := softTriggerTokensPolicy(coding, window)
	softChat := softTriggerTokensPolicy(chat, window)
	// Δreserved = 15000-4000 = 11000; soft = reserved + budget*0.70 + buf
	// → Δsoft = Δreserved * (1 - 0.70) = 3300.
	if diff := softCoding - softChat; diff != 3300 {
		t.Fatalf("soft trigger Δ(coding-chat_only) = %d, want 3300 (ToolsProfile-driven reserved)", diff)
	}

	// Regression: SnapshotMode must NOT influence reserved tokens.
	legacy := DefaultCompressPolicy()
	legacy.Model.SnapshotMode = "off"
	legacy.Profile.ToolsProfile = "coding"
	if got := softTriggerTokensPolicy(legacy, window); got != softCoding {
		t.Fatalf("SnapshotMode leaked into reserved computation: got %d want %d", got, softCoding)
	}

	hardCoding := hardTriggerTokensPolicy(coding, window)
	hardChat := hardTriggerTokensPolicy(chat, window)
	// Δhard = Δreserved * (1 - 0.90) = 1100.
	if diff := hardCoding - hardChat; diff != 1100 {
		t.Fatalf("hard trigger Δ(coding-chat_only) = %d, want 1100", diff)
	}
}

// --- G1: ctx 取消必须静默中止（不记瞬态失败、不写 hybrid 兜底标记） ---

// llmCallWithRetry 对 ctx 取消显式返回 fail=none（"ctx 取消不是压缩失败"），
// 但分块循环把 md=="" 一律映射为 transient：进程关闭/8 分钟超时会被误记入
// 失败抑制；hybrid 策略下更糟——取消时仍写兜底标记并推进 to_turn（事务在
// detach ctx 上提交），未摘要内容被永久跳过。
func TestLLMSummarize_CtxCancelAbortsWithoutFailure(t *testing.T) {
	read := &stubCompressReadDeps{}
	llm := &stubLLMCompressor{}
	c := newLLMTestCompressor(read, llm)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	msgs := []biz.ChatMessage{
		{Role: "user", TurnNumber: 1, ContentMarkdown: strings.Repeat("问", 300)},
		{Role: "assistant", TurnNumber: 1, ContentMarkdown: strings.Repeat("答", 300)},
	}
	for _, strategy := range []string{"summary", "hybrid"} {
		outcome := c.llmSummarize(ctx, biz.Session{}, biz.Agent{}, msgs, strategy, "sess-1")
		if outcome.fail != compressFailureNone {
			t.Fatalf("strategy %s: fail = %v, want none（ctx 取消不是压缩失败，不得记入抑制）", strategy, outcome.fail)
		}
		if outcome.markdown != "" {
			t.Fatalf("strategy %s: markdown = %q, want empty（取消不得写兜底标记/推进覆盖）", strategy, outcome.markdown)
		}
		if outcome.level != compressLevelNone {
			t.Fatalf("strategy %s: level = %v, want none", strategy, outcome.level)
		}
	}
	if len(llm.calls) != 0 {
		t.Fatalf("llm calls = %d, want 0（取消时不应发 LLM 调用）", len(llm.calls))
	}
}

// --- G2: 减量守卫分母必须计入被吸收的历史摘要 ---

// 滚动吸收场景：历史摘要 3000 runes（≈1200 tokens），本次 body 仅 ~219 runes
// （≈88 tokens）。LLM 产出 ≈217 runes（≈87 tokens）：相对 body 未达 80% 减量线，
// 但相对 (prior+body) 减量充分。守卫只比 body 时必然误杀成熟长会话的每次压缩
// （丢弃结果 + 记瞬态失败 → 上下文无限增长直到硬截断）。
func TestLLMSummarize_ReductionGuardCountsPriorSummary(t *testing.T) {
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 5, SummaryMarkdown: strings.Repeat("史", 3000)},
	}}
	llm := &stubLLMCompressor{}
	c := newLLMTestCompressor(read, llm)

	msgs := []biz.ChatMessage{
		{Role: "user", TurnNumber: 6, ContentMarkdown: strings.Repeat("问", 100)},
		{Role: "assistant", TurnNumber: 6, ContentMarkdown: strings.Repeat("答", 100)},
	}
	outcome := c.llmSummarize(context.Background(), biz.Session{}, biz.Agent{}, msgs, "summary", "sess-1")
	if outcome.level != compressLevelLLM || outcome.markdown == "" {
		t.Fatalf("outcome = %+v, want llm markdown（守卫不得误杀吸收大历史摘要的正常压缩）", outcome)
	}
}

// --- G3: cacheHit 必须聚合全块（部分命中不得谎称整次零 LLM 调用） ---

type stubCacheHitLLMCompressor struct {
	calls int
	hits  []bool // 1-based per-call cache-hit flags
}

func (s *stubCacheHitLLMCompressor) Compress(ctx context.Context, req compress.Request) (compress.Result, error) {
	res, _, err := s.CompressWithCacheHit(ctx, req)
	return res, err
}

func (s *stubCacheHitLLMCompressor) CompressWithCacheHit(_ context.Context, req compress.Request) (compress.Result, bool, error) {
	s.calls++
	hit := false
	if s.calls <= len(s.hits) {
		hit = s.hits[s.calls-1]
	}
	md := fmt.Sprintf("SUMMARY-CHUNK-%d\n\n%s", s.calls, strings.Repeat("已确认的事实与结论。", 20))
	return compress.Result{Markdown: md, Provider: req.Provider, Model: req.Model, PromptVersion: compress.PromptVersion}, hit, nil
}

func TestLLMSummarize_CacheHitAggregatesAcrossChunks(t *testing.T) {
	defer func(prev int) { compressChunkMaxRunes = prev }(compressChunkMaxRunes)
	compressChunkMaxRunes = 600 // force chunking: each msg ~200 runes → ~2 msgs/chunk

	mk := func() []biz.ChatMessage {
		var msgs []biz.ChatMessage
		for i := 1; i <= 8; i++ {
			msgs = append(msgs,
				biz.ChatMessage{Role: "user", TurnNumber: i, ContentMarkdown: strings.Repeat("问", 80)},
				biz.ChatMessage{Role: "assistant", TurnNumber: i, ContentMarkdown: strings.Repeat("答", 80)},
			)
		}
		return msgs
	}

	// 部分命中（首块未命中、后续命中）→ 必须聚合为 false。
	partial := &stubCacheHitLLMCompressor{hits: []bool{false, true, true, true}}
	c1 := newLLMTestCompressor(&stubCompressReadDeps{}, partial)
	out1 := c1.llmSummarize(context.Background(), biz.Session{}, biz.Agent{}, mk(), "summary", "sess-1")
	if out1.level != compressLevelLLM {
		t.Fatalf("partial: level = %v, want llm", out1.level)
	}
	if out1.cacheHit {
		t.Fatal("部分块命中时 cacheHit 必须为 false（元数据不得谎称整次压缩零 LLM 调用）")
	}

	// 全部命中 → true。
	all := &stubCacheHitLLMCompressor{hits: []bool{true, true, true, true}}
	c2 := newLLMTestCompressor(&stubCompressReadDeps{}, all)
	out2 := c2.llmSummarize(context.Background(), biz.Session{}, biz.Agent{}, mk(), "summary", "sess-2")
	if out2.level != compressLevelLLM {
		t.Fatalf("all: level = %v, want llm", out2.level)
	}
	if !out2.cacheHit {
		t.Fatal("全部块命中时 cacheHit 必须为 true")
	}
}

// --- G5: CAS 冲突不得记假成功 ---

// CAS 冲突（并发压缩已抢先写入）时 executeCompression 放弃写入，必须上报
// wrote=false；否则 runCompress 会清除失败抑制并打"压缩完成"假成功日志。
func TestExecuteCompression_CASConflictReportsNotWritten(t *testing.T) {
	read := &stubCompressReadDeps{}
	write := &stubCompressWriteDeps{read: read}
	tx := &stubCompressTxDeps{version: 7} // sess.CompressVersion=0 → oldVersion=7 ≠ 0 冲突
	c := &Compressor{
		deps: compressDeps{
			sessionReader:  read,
			messageReader:  read,
			summaryReader:  read,
			summaryWriter:  write,
			messageWriter:  write,
			contextUpdater: write,
			compressRepo:   tx,
		},
		lg: loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-1", CompressVersion: 0, RunnerSnapshotJSON: `{"state":{}}`}
	body := []biz.ChatMessage{{Role: "user", TurnNumber: 1, ContentMarkdown: "q"}}
	outcome := compressOutcome{level: compressLevelLLM, markdown: "摘要"}

	wrote, err := c.executeCompression(context.Background(), sess, biz.Agent{}, body, nil, outcome, "sess-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("CAS 冲突必须上报 wrote=false（不得记假成功）")
	}
}

// runCompress 级：CAS 冲突后既有的失败抑制记录必须保留（假成功会把它清掉）。
func TestRunCompress_CASConflictKeepsSuppression(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ID: "sess-1", ContextUsedTokens: 200000, LastContextWindowTokens: 256000, CompressVersion: 0},
		maxSummarized: 0,
		// 消息体必须足够大以通过减量守卫（stub 摘要 ~214 runes），
		// 让级联真实成功、走到写入阶段的 CAS 冲突点。
		msgs: func() []biz.ChatMessage {
			var out []biz.ChatMessage
			for t := 1; t <= 6; t++ {
				out = append(out,
					biz.ChatMessage{ID: "u" + string(rune('0'+t)), Role: "user", TurnNumber: t, ContentMarkdown: strings.Repeat("问", 300), CreatedAt: "2026-07-20T10:00:0" + string(rune('0'+t)) + "Z"},
					biz.ChatMessage{ID: "a" + string(rune('0'+t)), Role: "assistant", TurnNumber: t, ContentMarkdown: strings.Repeat("答", 300), CreatedAt: "2026-07-20T10:00:0" + string(rune('0'+t)) + "Z"},
				)
			}
			return out
		}(),
	}
	write := &stubCompressWriteDeps{read: read}
	tx := &stubCompressTxDeps{version: 9} // 与 sess.CompressVersion=0 冲突
	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: tx,
		},
		Compress: &stubLLMCompressor{},
		lg:       loggateway.NewNoop(),
		flight:   newCompressFlightManager(),
		buf:      newCompressBufferManager(),
		suppress: newCompressSuppressManager(),
	}
	// 预置一条瞬态抑制（模拟此前真实失败留下的退避）。
	c.suppress.record("sess-1", compressFailureTransient, "/", time.Now())

	// forced=true 绕过抑制检查，直达级联；CAS 冲突在写入阶段触发。
	if err := c.runCompress(context.Background(), "sess-1", "", cacheEnabledAgent(), true); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.suppress.m.Load("sess-1"); !ok {
		t.Fatal("CAS 冲突不得清除既有抑制记录（假成功会误导后续退避决策）")
	}
}
