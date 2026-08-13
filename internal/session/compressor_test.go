package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	bizsession "aranea-agents/internal/biz/session"
	"aranea-agents/internal/compress"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// --- narrow mocks for compressor deps ---

type stubCompressReadDeps struct {
	sess          biz.Session
	maxSummarized int
	msgs          []biz.ChatMessage
	listSummaries []biz.SessionSummary
	listErr       error
}

func (s *stubCompressReadDeps) SearchSessions(_ context.Context, _ biz.SessionSearchQuery) (biz.SessionListResult, error) {
	return biz.SessionListResult{}, nil
}
func (s *stubCompressReadDeps) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	sess := s.sess
	if sess.ID == "" {
		sess.ID = id
	}
	return sess, nil
}
func (s *stubCompressReadDeps) GetSessionRevision(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (s *stubCompressReadDeps) ListSessionsForBatch(_ context.Context, _ biz.SessionSearchQuery) ([]biz.Session, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) ListSessionsByIDs(_ context.Context, _ []string) ([]biz.Session, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) ListActiveAgentUserKeys(_ context.Context, _ int) ([]bizsession.AgentUserKey, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) CountMessagesBySession(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (s *stubCompressReadDeps) ListMessagesBySession(_ context.Context, _ string, _, _ int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) ListMessagesAfterTurn(_ context.Context, _ string, afterTurn int) ([]biz.ChatMessage, error) {
	var out []biz.ChatMessage
	for _, m := range s.msgs {
		if m.TurnNumber > afterTurn {
			out = append(out, m)
		}
	}
	return out, nil
}
func (s *stubCompressReadDeps) ListMessagesRecent(_ context.Context, _ string, _ int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) ListMessagesByIDs(_ context.Context, _ string, _ []string) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (s *stubCompressReadDeps) MaxSessionSummaryToTurn(_ context.Context, _ string) (int, error) {
	return s.maxSummarized, nil
}
func (s *stubCompressReadDeps) ListSessionSummaries(_ context.Context, _ string) ([]biz.SessionSummary, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.listSummaries, nil
}
func (s *stubCompressReadDeps) LatestSessionSummaryTime(_ context.Context, _ string) (string, error) {
	return "", nil
}

type stubCompressWriteDeps struct {
	read         *stubCompressReadDeps // Insert 后同步，模拟真实 DB 的读后可见
	inserted     []biz.SessionSummary
	deleted      bool
	existsResult bool
	snapshotJSON string
	estTokens    int
	window       int
}

func (s *stubCompressWriteDeps) AppendChatTurn(_ context.Context, _ string, _, _ biz.ChatMessage) error {
	return nil
}
func (s *stubCompressWriteDeps) AppendChatMessage(_ context.Context, _ string, _ biz.ChatMessage, _ bool) error {
	return nil
}
func (s *stubCompressWriteDeps) UpdateMessageFeedbackJSON(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (s *stubCompressWriteDeps) UpsertChatActivityMessage(_ context.Context, _ string, _ biz.ChatMessage) (bool, error) {
	return false, nil
}
func (s *stubCompressWriteDeps) InsertSessionSummary(_ context.Context, row biz.SessionSummary) error {
	s.inserted = append(s.inserted, row)
	if s.read != nil {
		s.read.listSummaries = append(s.read.listSummaries, row)
	}
	return nil
}
func (s *stubCompressWriteDeps) DeleteSessionSummaries(_ context.Context, _ string) error {
	s.deleted = true
	if s.read != nil {
		s.read.listSummaries = nil
	}
	return nil
}
func (s *stubCompressWriteDeps) UpdateSessionListSummary(_ context.Context, _, _ string) error {
	return nil
}
func (s *stubCompressWriteDeps) SessionSummaryExists(_ context.Context, _ string, _, _ int) (bool, error) {
	return s.existsResult, nil
}
func (s *stubCompressWriteDeps) UpdateRunnerSnapshotJSON(_ context.Context, _, raw string) error {
	s.snapshotJSON = raw
	return nil
}
func (s *stubCompressWriteDeps) UpdateSessionContextFromLLMUsage(_ context.Context, _ string, _, _, _ int) error {
	return nil
}
func (s *stubCompressWriteDeps) UpdateSessionContextAfterCompression(_ context.Context, _ string, est, win int) error {
	s.estTokens, s.window = est, win
	return nil
}
func (s *stubCompressWriteDeps) IncrementInvocationCounts(_ context.Context, _ string, _, _, _ int) error {
	return nil
}
func (s *stubCompressWriteDeps) ApplyMetricsDelta(_ context.Context, _ *bizsession.SessionMetricsDelta) error {
	return nil
}

type stubCompressTxDeps struct{ version int64 }

func (s *stubCompressTxDeps) TryIncrementCompressVersion(_ context.Context, _ string) (int64, error) {
	return s.version, nil
}
func (s *stubCompressTxDeps) CompressSessionInTx(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// makeTimeline builds alternating user/assistant messages, 2 per turn, turns 1..n.
func makeTimeline(turns int) []biz.ChatMessage {
	var out []biz.ChatMessage
	for t := 1; t <= turns; t++ {
		ts := "2026-07-20T10:00:0" + strconv.Itoa(t) + "Z"
		out = append(out,
			biz.ChatMessage{ID: "u" + strconv.Itoa(t), Role: "user", ContentMarkdown: "q" + strconv.Itoa(t), TurnNumber: t, CreatedAt: ts},
			biz.ChatMessage{ID: "a" + strconv.Itoa(t), Role: "assistant", ContentMarkdown: "ans" + strconv.Itoa(t), TurnNumber: t, CreatedAt: ts},
		)
	}
	return out
}

func newTestCompressor(read *stubCompressReadDeps) *Compressor {
	return &Compressor{
		deps: compressDeps{
			summaryReader: read,
			messageReader: read,
			sessionReader: read,
		},
		lg: loggateway.NewNoop(),
	}
}

func TestLoadCompressBody_returnsBodyAndTail(t *testing.T) {
	// 5 turns × 2 msgs = 10 rows; default keepTurns=4 → keepRows=8 → split=2.
	// body = turn 1 (2 rows), tail = turns 2..5 (8 rows).
	read := &stubCompressReadDeps{maxSummarized: 0, msgs: makeTimeline(5)}
	c := newTestCompressor(read)

	body, tail, _, cutoffTurn, err := c.loadCompressBody(context.Background(), biz.Session{}, biz.Agent{}, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if cutoffTurn != 1 {
		t.Fatalf("cutoffTurn = %d, want 1", cutoffTurn)
	}
	if len(body) != 2 {
		t.Fatalf("body len = %d, want 2", len(body))
	}
	if len(tail) != 8 {
		t.Fatalf("tail len = %d, want 8（近期轮次必须保留）", len(tail))
	}
	for _, m := range tail {
		if m.TurnNumber <= cutoffTurn {
			t.Fatalf("tail 中混入已压缩消息: %+v", m)
		}
	}
}

func TestExecuteCompression_tailWrittenIntoSnapshot(t *testing.T) {
	read := &stubCompressReadDeps{}
	write := &stubCompressWriteDeps{read: read}
	tx := &stubCompressTxDeps{version: 0}
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
	ag := biz.Agent{AgentKey: "agent-x"}
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "old q", TurnNumber: 1, CreatedAt: "2026-07-20T09:00:00Z"},
		{Role: "assistant", ContentMarkdown: "old a", TurnNumber: 1, CreatedAt: "2026-07-20T09:00:01Z"},
	}
	tail := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "recent question 保留", TurnNumber: 2, CreatedAt: "2026-07-20T10:00:00Z"},
		{Role: "assistant", ContentMarkdown: "recent answer 保留", TurnNumber: 2, CreatedAt: "2026-07-20T10:00:01Z"},
	}
	wrote, err := c.executeCompression(context.Background(), sess, ag, body, tail, compressOutcome{level: compressLevelLLM, markdown: "摘要内容"}, "sess-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("成功写入必须上报 wrote=true")
	}
	if write.snapshotJSON == "" {
		t.Fatal("snapshot 未写入")
	}
	if !strings.Contains(write.snapshotJSON, "recent question 保留") || !strings.Contains(write.snapshotJSON, "recent answer 保留") {
		t.Fatalf("tail 未写入快照: %s", write.snapshotJSON)
	}
	if !strings.Contains(write.snapshotJSON, "摘要内容") {
		t.Fatalf("摘要未写入快照: %s", write.snapshotJSON)
	}
}

// --- Phase 2: 递归滚动摘要 ---

type fakeLLMCompressor struct {
	lastReq compress.Request
	result  compress.Result
	err     error
}

func (f *fakeLLMCompressor) Compress(_ context.Context, req compress.Request) (compress.Result, error) {
	f.lastReq = req
	return f.result, f.err
}

func TestLLMCompress_passesPriorSummaryAndAbsorbs(t *testing.T) {
	read := &stubCompressReadDeps{}
	read.listSummaries = []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要A"},
		{FromTurn: 4, ToTurn: 5, SummaryMarkdown: "旧摘要B"},
	}
	fake := &fakeLLMCompressor{result: compress.Result{Markdown: "合并后的新摘要", Provider: "p", Model: "m"}}
	c := &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: fake,
		lg:       loggateway.NewNoop(),
	}
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "新问题", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "新回答", TurnNumber: 6},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if out.level != compressLevelLLM || out.markdown == "" {
		t.Fatalf("outcome = %+v", out)
	}
	if !strings.Contains(fake.lastReq.PriorSummary, "旧摘要A") || !strings.Contains(fake.lastReq.PriorSummary, "旧摘要B") {
		t.Fatalf("PriorSummary 未包含历史摘要: %q", fake.lastReq.PriorSummary)
	}
	if !out.absorbedPriors {
		t.Fatal("absorbedPriors 应为 true")
	}
}

func TestLLMCompress_hybridLLMSuccessAbsorbsPriors(t *testing.T) {
	// hybrid 策略下 LLM 成功产出真实摘要（非兜底标记）时，PriorSummary 已传入
	// 并被吸收——必须删除旧摘要行，否则内容重复拼接、无限增长。
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要"},
	}}
	fake := &fakeLLMCompressor{result: compress.Result{Markdown: "吸收合并后的新摘要", Provider: "p", Model: "m"}}
	c := &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: fake,
		lg:       loggateway.NewNoop(),
	}
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: "hybrid"}}
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "新问题", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "新回答", TurnNumber: 6},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, ag, body, nil, "sess-1")
	if out.level != compressLevelLLM || out.markdown == "" {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.absorbedPriors {
		t.Fatal("hybrid LLM 成功时 absorbedPriors 应为 true（摘要已吸收历史）")
	}
}

func TestLLMCompress_hybridFallbackMarkerKeepsPriors(t *testing.T) {
	// hybrid 策略下 LLM 失败 → 兜底标记不是吸收性摘要，必须保留旧摘要行。
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要"},
	}}
	fake := &fakeLLMCompressor{err: fmt.Errorf("boom")}
	c := &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: fake,
		lg:       loggateway.NewNoop(),
	}
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0TruncateStrategy: "hybrid"}}
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "新问题", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "新回答", TurnNumber: 6},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, ag, body, nil, "sess-1")
	if out.level != compressLevelLLM || out.markdown == "" {
		t.Fatalf("outcome = %+v", out)
	}
	if out.absorbedPriors {
		t.Fatal("hybrid 兜底标记时 absorbedPriors 应为 false（保留旧摘要）")
	}
}

func TestExecuteCompression_absorbDeletesPriors(t *testing.T) {
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要"},
	}}
	write := &stubCompressWriteDeps{read: read}
	tx := &stubCompressTxDeps{version: 0}
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
	outcome := compressOutcome{level: compressLevelLLM, markdown: "合并摘要", absorbedPriors: true}
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q", TurnNumber: 6, CreatedAt: "2026-07-20T11:00:00Z"}}
	wrote, err := c.executeCompression(context.Background(), sess, biz.Agent{}, body, nil, outcome, "sess-1", "u")
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("成功写入必须上报 wrote=true")
	}
	if !write.deleted {
		t.Fatal("旧摘要未被删除")
	}
	if len(write.inserted) != 1 || write.inserted[0].FromTurn != 1 {
		t.Fatalf("合并行 FromTurn 应为 1（吸收最早历史）: %+v", write.inserted)
	}
	if !strings.Contains(write.snapshotJSON, "合并摘要") {
		t.Fatalf("快照未含合并摘要: %s", write.snapshotJSON)
	}
	if strings.Contains(write.snapshotJSON, "旧摘要") {
		t.Fatalf("快照不应再拼接旧摘要: %s", write.snapshotJSON)
	}
}

func TestExecuteCompression_noAbsorbKeepsAppend(t *testing.T) {
	// absorbedPriors=false（如 hybrid 兜底 / L1 / L2）→ 不删除旧摘要，保持追加合并。
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 3, SummaryMarkdown: "旧摘要"},
	}}
	write := &stubCompressWriteDeps{read: read}
	tx := &stubCompressTxDeps{version: 0}
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
	outcome := compressOutcome{level: compressLevelLLM, markdown: "新摘要", absorbedPriors: false}
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q", TurnNumber: 6, CreatedAt: "2026-07-20T11:00:00Z"}}
	wrote, err := c.executeCompression(context.Background(), sess, biz.Agent{}, body, nil, outcome, "sess-1", "u")
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("成功写入必须上报 wrote=true")
	}
	if write.deleted {
		t.Fatal("未吸收时不应删除旧摘要")
	}
	if !strings.Contains(write.snapshotJSON, "旧摘要") || !strings.Contains(write.snapshotJSON, "新摘要") {
		t.Fatalf("快照应同时含旧摘要与新摘要（追加合并）: %s", write.snapshotJSON)
	}
}

// --- Phase 3: 摘要质量门（重试 / 退化检测 / 减量守卫 / 错误分类） ---

// scriptedLLMCompressor 按调用序返回脚本化结果：errs[i] 非 nil 优先返回 error。
type scriptedLLMCompressor struct {
	calls   int
	results []compress.Result
	errs    []error
}

func (s *scriptedLLMCompressor) Compress(_ context.Context, _ compress.Request) (compress.Result, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return compress.Result{}, s.errs[i]
	}
	if i < len(s.results) {
		return s.results[i], nil
	}
	return compress.Result{}, nil
}

func newQualityTestCompressor(fake *scriptedLLMCompressor) *Compressor {
	return &Compressor{
		deps:     compressDeps{summaryReader: &stubCompressReadDeps{}},
		Compress: fake,
		lg:       loggateway.NewNoop(),
	}
}

func TestLLMCompress_transientErrorThenSuccess(t *testing.T) {
	// 瞬态错误 → 重试 → 成功（errs/results 按调用序对齐：第 1 次 error，第 2 次 success）。
	fake := &scriptedLLMCompressor{
		errs:    []error{fmt.Errorf("boom"), nil},
		results: []compress.Result{{}, {Markdown: "有效摘要", Provider: "p", Model: "m"}},
	}
	c := newQualityTestCompressor(fake)
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "问题", TurnNumber: 1},
		{Role: "assistant", ContentMarkdown: "回答", TurnNumber: 1},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if out.level != compressLevelLLM || out.markdown != "有效摘要" {
		t.Fatalf("outcome = %+v", out)
	}
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want 2（瞬态错误应重试）", fake.calls)
	}
}

func TestLLMCompress_deterministicErrorNoRetry(t *testing.T) {
	// 确定性错误（鉴权失败）→ 不重试，直接返回 deterministic。
	authErr := apierror.Wrap(&trpcmodel.ResponseError{Message: "invalid api key", Type: "authentication_error"}, apierror.CodeInternal, apierror.DomainProvider)
	fake := &scriptedLLMCompressor{errs: []error{authErr, authErr, authErr}}
	c := newQualityTestCompressor(fake)
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "问题", TurnNumber: 1},
		{Role: "assistant", ContentMarkdown: "回答", TurnNumber: 1},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if out.level != compressLevelNone || out.fail != compressFailureDeterministic {
		t.Fatalf("outcome = %+v, want none+deterministic", out)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1（确定性错误不重试）", fake.calls)
	}
}

func TestLLMCompress_degenerateSummaryRetried(t *testing.T) {
	// transcript ≈ 1219 runes（≥1000 启用退化判定）；第一次短摘要退化 → 重试 → 第二次长摘要成功。
	fake := &scriptedLLMCompressor{
		results: []compress.Result{
			{Markdown: "短", Provider: "p", Model: "m"},
			{Markdown: strings.Repeat("摘", 300), Provider: "p", Model: "m"},
		},
	}
	c := newQualityTestCompressor(fake)
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: strings.Repeat("问", 600), TurnNumber: 1},
		{Role: "assistant", ContentMarkdown: strings.Repeat("答", 600), TurnNumber: 1},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if out.level != compressLevelLLM || utf8.RuneCountInString(out.markdown) != 300 {
		t.Fatalf("outcome level=%v md_runes=%d", out.level, utf8.RuneCountInString(out.markdown))
	}
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want 2（退化摘要应重试）", fake.calls)
	}
}

func TestLLMCompress_reductionGuardDrops(t *testing.T) {
	// transcript ≈ 1999 runes → bodyTokens=799；md 1800 runes → mdTokens=720 ≥ 0.8*799=639 → 守卫拦截。
	fake := &scriptedLLMCompressor{
		results: []compress.Result{
			{Markdown: strings.Repeat("摘", 1800), Provider: "p", Model: "m"},
			{Markdown: strings.Repeat("摘", 1800), Provider: "p", Model: "m"},
		},
	}
	c := newQualityTestCompressor(fake)
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: strings.Repeat("问", 990), TurnNumber: 1},
		{Role: "assistant", ContentMarkdown: strings.Repeat("答", 990), TurnNumber: 1},
	}
	out := c.llmCompress(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if out.level != compressLevelNone || out.fail != compressFailureTransient {
		t.Fatalf("outcome = %+v, want none+transient（减量不足应丢弃）", out)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1（守卫在重试循环外，首次即拦截）", fake.calls)
	}
}

// --- Phase 4: 压缩失败抑制（runCompress 行为） ---

func newSuppressTestCompressor(read *stubCompressReadDeps, fake *scriptedLLMCompressor) *Compressor {
	return &Compressor{
		deps:     compressDeps{sessionReader: read, messageReader: read, summaryReader: read},
		Compress: fake,
		lg:       loggateway.NewNoop(),
		flight:   newCompressFlightManager(),
		buf:      newCompressBufferManager(),
		suppress: newCompressSuppressManager(),
	}
}

func TestRunCompress_suppressedAfterDeterministicFailure(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ID: "sess-1", ContextUsedTokens: 100000, LastContextWindowTokens: 1000},
		maxSummarized: 0,
		msgs:          makeTimeline(6),
	}
	authErr := apierror.Wrap(&trpcmodel.ResponseError{Message: "invalid api key", Type: "authentication_error"}, apierror.CodeInternal, apierror.DomainProvider)
	fake := &scriptedLLMCompressor{errs: []error{authErr, authErr, authErr}}
	c := newSuppressTestCompressor(read, fake)
	ag := biz.Agent{}

	// 第一次：触发压缩，LLM 确定性失败 → 记录抑制 + 返回 error。
	if err := c.runCompress(context.Background(), "sess-1", "u", ag, false); err == nil {
		t.Fatal("确定性失败应返回 error")
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	// 第二次：同模型被抑制，不再调用 LLM，返回 nil（未尝试压缩）。
	if err := c.runCompress(context.Background(), "sess-1", "u", ag, false); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("抑制后仍调用 LLM: calls = %d, want 1", fake.calls)
	}
}

func TestRunCompress_forcedBypassesSuppression(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ID: "sess-1", ContextUsedTokens: 100000, LastContextWindowTokens: 1000},
		maxSummarized: 0,
		msgs:          makeTimeline(6),
	}
	authErr := apierror.Wrap(&trpcmodel.ResponseError{Message: "invalid api key", Type: "authentication_error"}, apierror.CodeInternal, apierror.DomainProvider)
	fake := &scriptedLLMCompressor{errs: []error{authErr, authErr, authErr}}
	c := newSuppressTestCompressor(read, fake)
	ag := biz.Agent{}

	// 第一次：确定性失败 → 记录抑制 + 返回 error。
	if err := c.runCompress(context.Background(), "sess-1", "u", ag, false); err == nil {
		t.Fatal("确定性失败应返回 error")
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	// forced=true（手动 /compact、durable turn）：绕过抑制，再次调用 LLM（仍失败）。
	if err := c.runCompress(context.Background(), "sess-1", "u", ag, true); err == nil {
		t.Fatal("forced 仍遇确定性失败，应返回 error")
	}
	if fake.calls != 2 {
		t.Fatalf("forced 应绕过抑制: calls = %d, want 2", fake.calls)
	}
}

// --- 摘要行数累积上限（超限强制 L3） ---

func newCascadeTestCompressor(read *stubCompressReadDeps, fake *scriptedLLMCompressor) *Compressor {
	return &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: fake,
		// ICS 门控（memoryCompactMinICS=0.5）要求事实集覆盖足够维度：
		// intent(0.25)+state(0.20)+decision×1(0.10)=0.55 → L2 可产出。
		memoryReader: &stubMemoryFactReader{facts: []biz.MemoryFactEntry{
			{Statement: "用户意图", Scope: "intent"},
			{Statement: "当前状态", Scope: "state"},
			{Statement: "已做决策", Scope: "decision"},
		}},
		lg: loggateway.NewNoop(),
	}
}

func TestCompressCascade_summaryRowsExceededForcesLLM(t *testing.T) {
	// 3 行历史摘要 = 默认上限：L2 本可成功，但行数超限必须强制 L3（LLM 吸收合并归一）。
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 2, SummaryMarkdown: "摘要1"},
		{FromTurn: 3, ToTurn: 4, SummaryMarkdown: "摘要2"},
		{FromTurn: 5, ToTurn: 5, SummaryMarkdown: "摘要3"},
	}}
	fake := &scriptedLLMCompressor{results: []compress.Result{{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"}}}
	c := newCascadeTestCompressor(read, fake)
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "q6", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "a6", TurnNumber: 6},
	}
	out := c.compressCascade(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1", 5, 0, 0)
	if out.level != compressLevelLLM {
		t.Fatalf("level = %v, want LLM（行数超限应强制 L3）", out.level)
	}
	if fake.calls != 1 {
		t.Fatalf("LLM calls = %d, want 1", fake.calls)
	}
	if !out.absorbedPriors {
		t.Fatal("强制 L3 应吸收历史摘要（absorbedPriors=true）")
	}
}

func TestCompressCascade_summaryRowsBelowCapKeepsL2(t *testing.T) {
	// 1 行历史摘要 < 上限：走 L2 MemoryCompact，不调用 LLM。
	read := &stubCompressReadDeps{listSummaries: []biz.SessionSummary{
		{FromTurn: 1, ToTurn: 2, SummaryMarkdown: "摘要1"},
	}}
	fake := &scriptedLLMCompressor{results: []compress.Result{{Markdown: "不应被调用"}}}
	c := newCascadeTestCompressor(read, fake)
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q6", TurnNumber: 6}}
	out := c.compressCascade(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1", 5, 0, 0)
	if out.level != compressLevelMemory {
		t.Fatalf("level = %v, want Memory（行数未超限应走 L2）", out.level)
	}
	if fake.calls != 0 {
		t.Fatalf("LLM 不应被调用: calls = %d", fake.calls)
	}
}

func TestCompressCascade_summaryRowsReadErrorFallsBackToL2(t *testing.T) {
	// 读取摘要行数失败：非致命，按未超限处理走 L2。
	read := &stubCompressReadDeps{listErr: fmt.Errorf("db down")}
	fake := &scriptedLLMCompressor{results: []compress.Result{{Markdown: "不应被调用"}}}
	c := newCascadeTestCompressor(read, fake)
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q6", TurnNumber: 6}}
	out := c.compressCascade(context.Background(), biz.Session{}, biz.Agent{}, body, nil, "sess-1", 5, 0, 0)
	if out.level != compressLevelMemory {
		t.Fatalf("level = %v, want Memory（读失败应回退 L2）", out.level)
	}
	if fake.calls != 0 {
		t.Fatalf("LLM 不应被调用: calls = %d", fake.calls)
	}
}

// --- LLM 压缩缓存接线（session 隔离 + per-agent 开关 + cache_hit 透传） ---

type stubMonitorBus struct {
	events []contract.MonitorEvent
}

func (s *stubMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	s.events = append(s.events, ev)
}
func (s *stubMonitorBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	ch := make(chan contract.MonitorEvent)
	return ch, func() {}
}
func (s *stubMonitorBus) DropCount() uint64 { return 0 }

// newCacheTestCompressor builds a Compressor with the full runCompress success-path
// mock chain (read/write/tx) and a CachingCompressor wrapping the fake inner LLM.
// write 不联动 read：priorSummary 恒为空，相同 session 的重复压缩缓存键稳定。
func newCacheTestCompressor(read *stubCompressReadDeps, write *stubCompressWriteDeps, inner compress.Compressor, cache *compress.CompressCache) *Compressor {
	return &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: &stubCompressTxDeps{version: 0},
		},
		Compress: compress.NewCachingCompressor(inner, cache, loggateway.NewNoop()),
		lg:       loggateway.NewNoop(),
		flight:   newCompressFlightManager(),
		buf:      newCompressBufferManager(),
		suppress: newCompressSuppressManager(),
	}
}

func cacheEnabledAgent() biz.Agent {
	// MemoryCompactEnabled=false：级联跳过 L2 直达 L3，聚焦缓存行为断言。
	return biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressLLMCacheEnabled: true}}
}

func TestRunCompress_CacheEnabledHitsOnRepeat(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ID: "sess-1", ContextUsedTokens: 100000, LastContextWindowTokens: 1000},
		maxSummarized: 0,
		msgs:          makeTimeline(6),
	}
	write := &stubCompressWriteDeps{}
	fake := &scriptedLLMCompressor{results: []compress.Result{{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"}}}
	cache := compress.NewCompressCache(16, 10*time.Minute, loggateway.NewNoop())
	c := newCacheTestCompressor(read, write, fake, cache)
	ag := cacheEnabledAgent()

	if err := c.runCompress(context.Background(), "sess-1", "u", ag, true); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("首次压缩 calls = %d, want 1", fake.calls)
	}
	// 相同 session + 相同输入（stub 静态数据）→ 第二次必须命中缓存。
	if err := c.runCompress(context.Background(), "sess-1", "u", ag, true); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 {
		t.Fatalf("相同输入重复压缩应命中缓存: calls = %d, want 1", fake.calls)
	}
}

func TestRunCompress_CacheIsolatesSessions(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ContextUsedTokens: 100000, LastContextWindowTokens: 1000}, // ID 由 GetSessionByID 按请求填充
		maxSummarized: 0,
		msgs:          makeTimeline(6),
	}
	write := &stubCompressWriteDeps{}
	fake := &scriptedLLMCompressor{results: []compress.Result{
		{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"},
		{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"},
	}}
	cache := compress.NewCompressCache(16, 10*time.Minute, loggateway.NewNoop())
	c := newCacheTestCompressor(read, write, fake, cache)
	ag := cacheEnabledAgent()

	if err := c.runCompress(context.Background(), "sess-1", "u", ag, true); err != nil {
		t.Fatal(err)
	}
	if err := c.runCompress(context.Background(), "sess-2", "u", ag, true); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 2 {
		t.Fatalf("不同会话相同输入不得共享缓存: calls = %d, want 2（sessionID 必须注入缓存键）", fake.calls)
	}
}

func TestRunCompress_CacheDisabledSkipsCache(t *testing.T) {
	read := &stubCompressReadDeps{
		sess:          biz.Session{ID: "sess-1", ContextUsedTokens: 100000, LastContextWindowTokens: 1000},
		maxSummarized: 0,
		msgs:          makeTimeline(6),
	}
	write := &stubCompressWriteDeps{}
	fake := &scriptedLLMCompressor{results: []compress.Result{
		{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"},
		{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"},
	}}
	cache := compress.NewCompressCache(16, 10*time.Minute, loggateway.NewNoop())
	c := newCacheTestCompressor(read, write, fake, cache)
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressLLMCacheEnabled: false}}

	for i := 0; i < 2; i++ {
		if err := c.runCompress(context.Background(), "sess-1", "u", ag, true); err != nil {
			t.Fatal(err)
		}
	}
	if fake.calls != 2 {
		t.Fatalf("缓存禁用时不应命中: calls = %d, want 2", fake.calls)
	}
	if cache.Len() != 0 {
		t.Fatalf("缓存禁用时不应写入: len = %d, want 0", cache.Len())
	}
}

func TestLLMCompress_CacheHitSetsOutcomeFlag(t *testing.T) {
	read := &stubCompressReadDeps{}
	fake := &scriptedLLMCompressor{results: []compress.Result{{Markdown: "LLM 合并摘要", Provider: "p", Model: "m"}}}
	cache := compress.NewCompressCache(16, 10*time.Minute, loggateway.NewNoop())
	c := &Compressor{
		deps:     compressDeps{summaryReader: read},
		Compress: compress.NewCachingCompressor(fake, cache, loggateway.NewNoop()),
		lg:       loggateway.NewNoop(),
	}
	ctx := compress.ContextWithSessionID(context.Background(), "sess-1")
	body := []biz.ChatMessage{
		{Role: "user", ContentMarkdown: "q6", TurnNumber: 6},
		{Role: "assistant", ContentMarkdown: "a6", TurnNumber: 6},
	}
	first := c.llmCompress(ctx, biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if first.level != compressLevelLLM || first.cacheHit {
		t.Fatalf("首次应 miss: level=%v cacheHit=%v", first.level, first.cacheHit)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	second := c.llmCompress(ctx, biz.Session{}, biz.Agent{}, body, nil, "sess-1")
	if second.level != compressLevelLLM || !second.cacheHit {
		t.Fatalf("第二次应命中: level=%v cacheHit=%v", second.level, second.cacheHit)
	}
	if fake.calls != 1 {
		t.Fatalf("命中不应再调 LLM: calls = %d, want 1", fake.calls)
	}
}

func TestExecuteCompression_CacheHitInNoticeMetadata(t *testing.T) {
	read := &stubCompressReadDeps{}
	write := &stubCompressWriteDeps{}
	bus := &stubMonitorBus{}
	c := &Compressor{
		deps: compressDeps{
			sessionReader: read, messageReader: read, summaryReader: read,
			summaryWriter: write, messageWriter: write, contextUpdater: write,
			compressRepo: &stubCompressTxDeps{version: 0},
		},
		monitorBus: bus,
		lg:         loggateway.NewNoop(),
	}
	sess := biz.Session{ID: "sess-1", CompressVersion: 0, RunnerSnapshotJSON: `{"state":{}}`}
	outcome := compressOutcome{level: compressLevelLLM, markdown: "合并摘要", cacheHit: true}
	body := []biz.ChatMessage{{Role: "user", ContentMarkdown: "q", TurnNumber: 6}}
	wrote, err := c.executeCompression(context.Background(), sess, biz.Agent{}, body, nil, outcome, "sess-1", "u")
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("成功写入必须上报 wrote=true")
	}
	if len(bus.events) != 1 {
		t.Fatalf("应发布 1 条压缩通知事件, got %d", len(bus.events))
	}
	hit, ok := bus.events[0].Metadata["cache_hit"].(bool)
	if !ok || !hit {
		t.Fatalf("metadata cache_hit 应为 true: %v", bus.events[0].Metadata)
	}
}
