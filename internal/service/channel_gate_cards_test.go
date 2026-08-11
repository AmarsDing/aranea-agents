package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// === stub ===

type stubGateCardStepReader struct {
	steps map[string]biz.Step
}

func (r *stubGateCardStepReader) GetStep(_ context.Context, id string) (biz.Step, error) {
	if s, ok := r.steps[id]; ok {
		return s, nil
	}
	return biz.Step{}, errors.New("not found")
}

func (r *stubGateCardStepReader) ListStepsByTurn(context.Context, string) ([]biz.Step, error) {
	return nil, nil
}
func (r *stubGateCardStepReader) ListStepsByTask(context.Context, string) ([]biz.Step, error) {
	return nil, nil
}
func (r *stubGateCardStepReader) ListStepsBySession(context.Context, string) ([]biz.Step, error) {
	return nil, nil
}
func (r *stubGateCardStepReader) ListStepsBySessionPaged(context.Context, string, biz.StepListOptions) ([]biz.Step, bool, error) {
	return nil, false, nil
}
func (r *stubGateCardStepReader) ListStepsBySpiritSession(context.Context, string) ([]biz.Step, error) {
	return nil, nil
}
func (r *stubGateCardStepReader) ListStepsBySessionID(context.Context, string) ([]biz.Step, error) {
	return nil, nil
}
func (r *stubGateCardStepReader) MaxSeqBySpiritSession(context.Context, string) (int64, error) {
	return 0, nil
}

type stubGateCardChat struct {
	confirmAccepted bool
	confirmReply    string
	confirmCalls    int
	lastReplyToken  string

	clarifyReply   string
	clarifyErr     error
	clarifyCalls   int
	lastAnswers    []biz.ClarificationAnswer
	lastClarifySid string
}

func (g *stubGateCardChat) ConfirmToolGateForCard(_ context.Context, sessionID, stepID, replyToken string) (bool, string) {
	g.confirmCalls++
	g.lastReplyToken = replyToken
	return g.confirmAccepted, g.confirmReply
}

func (g *stubGateCardChat) SubmitClarificationForCard(_ context.Context, sessionID, stepID string, answers []biz.ClarificationAnswer) (string, error) {
	g.clarifyCalls++
	g.lastAnswers = answers
	g.lastClarifySid = sessionID
	return g.clarifyReply, g.clarifyErr
}

func newTestGateCards(steps map[string]biz.Step, chat *stubGateCardChat) *ChannelGateCards {
	return NewChannelGateCards(nil, &biz.SessionUsecase{}, &biz.ChannelUsecase{}, chat,
		&stubGateCardStepReader{steps: steps}, loggateway.NewNoop())
}

func clarifyStepContent(t *testing.T, qs []biz.ClarificationQuestion) string {
	t.Helper()
	raw, err := json.Marshal(biz.ClarificationEnvelope{
		Version:   1,
		Kind:      biz.ClarificationEnvelopeKind,
		Questions: qs,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return string(raw)
}

// === 纯函数 ===

func TestGateCardStepBelongs(t *testing.T) {
	cases := []struct {
		name          string
		step          biz.Step
		cardSessionID string
		want          bool
	}{
		{"direct", biz.Step{SessionID: "s1"}, "s1", true},
		{"spirit fallback", biz.Step{SessionID: "member-1", SpiritSessionID: "root-1"}, "root-1", true},
		{"reject other session", biz.Step{SessionID: "s1"}, "s2", false},
		{"reject empty card session", biz.Step{SessionID: "s1"}, "", false},
		{"reject blank spirit", biz.Step{SessionID: "member-1", SpiritSessionID: " "}, "root-1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gateCardStepBelongs(tc.step, tc.cardSessionID); got != tc.want {
				t.Fatalf("gateCardStepBelongs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGateCardConfirmReplyToken(t *testing.T) {
	for _, key := range []string{"approve", "deny", "approve_session", "approve_always", " Approve "} {
		if got := gateCardConfirmReplyToken(key); got == "" {
			t.Fatalf("gateCardConfirmReplyToken(%q) = empty, want non-empty", key)
		}
	}
	for _, bad := range []string{"", "yes", "allow", "approved"} {
		if got := gateCardConfirmReplyToken(bad); got != "" {
			t.Fatalf("gateCardConfirmReplyToken(%q) = %q, want empty", bad, got)
		}
	}
}

func TestGateCardClarifyInteractive(t *testing.T) {
	single := func(nOpts int) biz.ClarificationQuestion {
		opts := make([]string, nOpts)
		for i := range opts {
			opts[i] = "opt"
		}
		return biz.ClarificationQuestion{Question: "q", Mode: biz.ClarificationModeSingle, Options: opts}
	}
	cases := []struct {
		name string
		qs   []biz.ClarificationQuestion
		want bool
	}{
		{"single ok", []biz.ClarificationQuestion{single(2)}, true},
		{"empty questions", nil, false},
		{"multi mode degrades", []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeMulti, Options: []string{"a"}}}, false},
		{"no options degrades", []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeSingle}}, false},
		{"too many options degrades", []biz.ClarificationQuestion{single(gateCardMaxOptions + 1)}, false},
		{"too many questions degrades", make([]biz.ClarificationQuestion, gateCardMaxQuestions+1), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := gateCardClarifyInteractive(tc.qs); got != tc.want {
				t.Fatalf("gateCardClarifyInteractive() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGateCardReceiveTarget(t *testing.T) {
	cases := []struct {
		name     string
		meta     biz.ChannelSessionMeta
		wantRec  string
		wantType string
	}{
		{"chat id prefix", biz.ChannelSessionMeta{PeerID: "oc_abc"}, "oc_abc", "chat_id"},
		{"open id", biz.ChannelSessionMeta{PeerID: "ou_xyz"}, "ou_xyz", "open_id"},
		{"peer key fallback", biz.ChannelSessionMeta{PeerKey: "ou_k"}, "ou_k", "open_id"},
		{"empty", biz.ChannelSessionMeta{}, "", "open_id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, typ := gateCardReceiveTarget(tc.meta)
			if rec != tc.wantRec || typ != tc.wantType {
				t.Fatalf("gateCardReceiveTarget() = (%q, %q), want (%q, %q)", rec, typ, tc.wantRec, tc.wantType)
			}
		})
	}
}

// === HandleConfirmClick ===

func TestHandleConfirmClick_StepNotFound(t *testing.T) {
	m := newTestGateCards(map[string]biz.Step{}, &stubGateCardChat{})
	if got := m.HandleConfirmClick(context.Background(), "s1", "missing", "approve"); got != "确认不存在或已删除" {
		t.Fatalf("got %q", got)
	}
}

func TestHandleConfirmClick_RejectForeignSession(t *testing.T) {
	steps := map[string]biz.Step{
		"st1": {ID: "st1", SessionID: "s1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked},
	}
	chat := &stubGateCardChat{confirmAccepted: true, confirmReply: "已批准执行"}
	m := newTestGateCards(steps, chat)
	if got := m.HandleConfirmClick(context.Background(), "s2", "st1", "approve"); got != "确认不属于当前会话" {
		t.Fatalf("got %q", got)
	}
	if chat.confirmCalls != 0 {
		t.Fatalf("gateway must not be called for foreign session, got %d calls", chat.confirmCalls)
	}
}

func TestHandleConfirmClick_SpiritSessionFallback(t *testing.T) {
	steps := map[string]biz.Step{
		"st1": {ID: "st1", SessionID: "member-1", SpiritSessionID: "root-1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked},
	}
	chat := &stubGateCardChat{confirmAccepted: true, confirmReply: "已批准执行"}
	m := newTestGateCards(steps, chat)
	got := m.HandleConfirmClick(context.Background(), "root-1", "st1", "approve")
	if got != "已批准执行" {
		t.Fatalf("got %q", got)
	}
	if chat.confirmCalls != 1 {
		t.Fatalf("gateway calls = %d, want 1", chat.confirmCalls)
	}
}

func TestHandleConfirmClick_UnknownReplyKey(t *testing.T) {
	steps := map[string]biz.Step{
		"st1": {ID: "st1", SessionID: "s1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked},
	}
	chat := &stubGateCardChat{}
	m := newTestGateCards(steps, chat)
	if got := m.HandleConfirmClick(context.Background(), "s1", "st1", "bogus"); got != "未知的确认操作" {
		t.Fatalf("got %q", got)
	}
	if chat.confirmCalls != 0 {
		t.Fatalf("gateway must not be called for unknown reply key")
	}
}

func TestHandleConfirmClick_StaleClosesCard(t *testing.T) {
	// gateway 拒绝（已处理/已超时）→ 卡片应被置灰并移除跟踪。
	steps := map[string]biz.Step{
		"st1": {ID: "st1", SessionID: "s1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked},
	}
	chat := &stubGateCardChat{confirmAccepted: false, confirmReply: "该确认已被处理或已超时"}
	m := newTestGateCards(steps, chat)
	m.track(&gateCardRef{stepID: "st1", sessionID: "s1", kind: biz.StepKindConfirm}) // 无 messageID → 不 PATCH 但移除跟踪
	got := m.HandleConfirmClick(context.Background(), "s1", "st1", "approve")
	if got != "该确认已被处理或已超时" {
		t.Fatalf("got %q", got)
	}
	if m.isTracked("st1") {
		t.Fatalf("stale ref must be untracked after rejected confirm")
	}
}

// === SelectClarifyOption ===

func newClarifyStep(t *testing.T, id, sessionID string, status biz.StepStatus, qs []biz.ClarificationQuestion) biz.Step {
	t.Helper()
	return biz.Step{
		ID:        id,
		SessionID: sessionID,
		Kind:      biz.StepKindClarify,
		Status:    status,
		Content:   clarifyStepContent(t, qs),
	}
}

func TestSelectClarifyOption_NotAwaiting(t *testing.T) {
	qs := []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeSingle, Options: []string{"a"}}}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusCompleted, qs),
	}
	m := newTestGateCards(steps, &stubGateCardChat{})
	m.track(&gateCardRef{stepID: "st1", sessionID: "s1", kind: biz.StepKindClarify})
	got := m.SelectClarifyOption(context.Background(), "s1", "st1", 0, "a")
	if got != "该澄清已提交或已失效" {
		t.Fatalf("got %q", got)
	}
	if m.isTracked("st1") {
		t.Fatalf("stale ref must be untracked")
	}
}

func TestSelectClarifyOption_QuestionIndexOutOfRange(t *testing.T) {
	qs := []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeSingle, Options: []string{"a"}}}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusAwaitingInput, qs),
	}
	m := newTestGateCards(steps, &stubGateCardChat{})
	if got := m.SelectClarifyOption(context.Background(), "s1", "st1", 5, "a"); got != "未知的问题" {
		t.Fatalf("got %q", got)
	}
	if got := m.SelectClarifyOption(context.Background(), "s1", "st1", -1, "a"); got != "未知的问题" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectClarifyOption_MultiModeRejected(t *testing.T) {
	qs := []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeMulti, Options: []string{"a", "b"}}}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusAwaitingInput, qs),
	}
	m := newTestGateCards(steps, &stubGateCardChat{})
	if got := m.SelectClarifyOption(context.Background(), "s1", "st1", 0, "a"); got != "该问题支持多选，请直接回复文字作答" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectClarifyOption_UnknownOption(t *testing.T) {
	qs := []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeSingle, Options: []string{"a", "b"}}}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusAwaitingInput, qs),
	}
	m := newTestGateCards(steps, &stubGateCardChat{})
	if got := m.SelectClarifyOption(context.Background(), "s1", "st1", 0, "zzz"); got != "未知的选项" {
		t.Fatalf("got %q", got)
	}
}

func TestSelectClarifyOption_PartialThenAutoSubmit(t *testing.T) {
	qs := []biz.ClarificationQuestion{
		{Question: "q1", Mode: biz.ClarificationModeSingle, Options: []string{"a", "b"}},
		{Question: "q2", Mode: biz.ClarificationModeSingle, Options: []string{"x", "y"}},
	}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusAwaitingInput, qs),
	}
	chat := &stubGateCardChat{clarifyReply: "已提交澄清回答"}
	m := newTestGateCards(steps, chat)

	// 第一题：记录但不提交（瞬态 ref，无 messageID → 不 PATCH）。
	got := m.SelectClarifyOption(context.Background(), "s1", "st1", 0, "a")
	if got != "已记录第 1 题选择（1/2）" {
		t.Fatalf("got %q", got)
	}
	if chat.clarifyCalls != 0 {
		t.Fatalf("submit must not fire before all answered")
	}

	// 第二题：全部作答 → 自动提交。
	got = m.SelectClarifyOption(context.Background(), "s1", "st1", 1, "y")
	if got != "已提交澄清回答" {
		t.Fatalf("got %q", got)
	}
	if chat.clarifyCalls != 1 {
		t.Fatalf("clarifyCalls = %d, want 1", chat.clarifyCalls)
	}
	if chat.lastClarifySid != "s1" {
		t.Fatalf("submit session = %q, want s1", chat.lastClarifySid)
	}
	if len(chat.lastAnswers) != 2 || chat.lastAnswers[0].Selected[0] != "a" || chat.lastAnswers[1].Selected[0] != "y" {
		t.Fatalf("answers = %+v", chat.lastAnswers)
	}
}

func TestSelectClarifyOption_ReselectOverwrites(t *testing.T) {
	qs := []biz.ClarificationQuestion{
		{Question: "q1", Mode: biz.ClarificationModeSingle, Options: []string{"a", "b"}},
	}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusAwaitingInput, qs),
	}
	chat := &stubGateCardChat{clarifyReply: "已提交澄清回答"}
	m := newTestGateCards(steps, chat)

	m.SelectClarifyOption(context.Background(), "s1", "st1", 0, "a")
	// 改选 b：应覆盖 a 并提交 b。
	m.SelectClarifyOption(context.Background(), "s1", "st1", 0, "b")
	if chat.clarifyCalls != 2 {
		t.Fatalf("clarifyCalls = %d, want 2 (每次全答都尝试提交，状态机兜底幂等)", chat.clarifyCalls)
	}
	if chat.lastAnswers[0].Selected[0] != "b" {
		t.Fatalf("last answer = %+v, want b", chat.lastAnswers[0].Selected)
	}
}

func TestSelectClarifyOption_ForeignSessionRejected(t *testing.T) {
	qs := []biz.ClarificationQuestion{{Question: "q", Mode: biz.ClarificationModeSingle, Options: []string{"a"}}}
	steps := map[string]biz.Step{
		"st1": newClarifyStep(t, "st1", "s1", biz.StepStatusAwaitingInput, qs),
	}
	chat := &stubGateCardChat{}
	m := newTestGateCards(steps, chat)
	if got := m.SelectClarifyOption(context.Background(), "s2", "st1", 0, "a"); got != "澄清不属于当前会话" {
		t.Fatalf("got %q", got)
	}
	if chat.clarifyCalls != 0 {
		t.Fatalf("gateway must not be called for foreign session")
	}
}

// === maybeOpenGate 幂等 ===

func TestMaybeOpenGate_SkipsTrackedAndIrrelevant(t *testing.T) {
	m := newTestGateCards(map[string]biz.Step{}, &stubGateCardChat{})
	m.track(&gateCardRef{stepID: "st1", sessionID: "s1", kind: biz.StepKindConfirm})
	// 已跟踪 → openGate 不被触发（resolveChannelMeta 会因零值 SessionUsecase panic 或直接返回；
	// 此测试断言早退路径不会触碰 sessions）。
	m.maybeOpenGate(context.Background(), biz.Step{ID: "st1", Kind: biz.StepKindConfirm, Status: biz.StepStatusToolBlocked})
	// 无关 kind/status → 早退。
	m.maybeOpenGate(context.Background(), biz.Step{ID: "st2", Kind: biz.StepKindReply, Status: biz.StepStatusCompleted})
	m.maybeOpenGate(context.Background(), biz.Step{ID: "st3", Kind: biz.StepKindConfirm, Status: biz.StepStatusCompleted})
}

// === resultCard ===

func TestResultCard_ConfirmOutcomes(t *testing.T) {
	m := newTestGateCards(map[string]biz.Step{}, &stubGateCardChat{})
	ref := &gateCardRef{stepID: "st1", sessionID: "s1", kind: biz.StepKindConfirm, toolName: "shell_exec"}

	// approved via notice
	card, ok := m.resultCard(context.Background(), ref, nil, "approved")
	if !ok || card == "" {
		t.Fatalf("approved result card missing")
	}
	assertCardHeader(t, card, "green")

	// rejected via notice
	card, ok = m.resultCard(context.Background(), ref, nil, "rejected")
	if !ok {
		t.Fatalf("rejected result card missing")
	}
	assertCardHeader(t, card, "red")

	// timeout via step cancelled + confirm_timeout
	now := time.Now().UTC()
	step := &biz.Step{ID: "st1", Status: biz.StepStatusCancelled, ToolErrorCode: gateCardConfirmTimeoutCode, CompletedAt: &now}
	card, ok = m.resultCard(context.Background(), ref, step, "")
	if !ok {
		t.Fatalf("timeout result card missing")
	}
	assertCardHeader(t, card, "grey")

	// 未终态 → 不产卡
	step = &biz.Step{ID: "st1", Status: biz.StepStatusToolBlocked}
	if _, ok := m.resultCard(context.Background(), ref, step, ""); ok {
		t.Fatalf("non-terminal step must not produce result card")
	}
}

func assertCardHeader(t *testing.T, cardJSON, wantTemplate string) {
	t.Helper()
	var card struct {
		Header struct {
			Template string `json:"template"`
		} `json:"header"`
	}
	if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
		t.Fatalf("card JSON unparseable: %v", err)
	}
	if card.Header.Template != wantTemplate {
		t.Fatalf("template = %q, want %q", card.Header.Template, wantTemplate)
	}
}
