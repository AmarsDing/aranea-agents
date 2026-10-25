package agent

// D4-a/D4-b 档位 provider 钉板（就绪不翻）。本文件所有用例共享进程级
// provider 全局态，一律不 t.Parallel，且通过 reset helpers 在用例间复位。
//
// 翻牌成对契约：prompt 级与 message 级同开关一起切；segmented 档偏差记档
// （87 design）——Σ per-message ceil 恒 ≥ 整片 ceil，差 ≤ N-1（保守方向）。

import (
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// estGateCaptureLogger 复用 captureEntry/zapFieldsToMap（prompt_est_tokens_test.go），
// 另补 Warn 捕获面供 once-Warn 断言。
type estGateCaptureLogger struct {
	infos []captureEntry
	warns []string
}

func (l *estGateCaptureLogger) Debug(string, ...loggateway.Field) {}
func (l *estGateCaptureLogger) Info(msg string, fields ...loggateway.Field) {
	l.infos = append(l.infos, captureEntry{msg: msg, fields: zapFieldsToMap(fields)})
}
func (l *estGateCaptureLogger) Warn(msg string, fields ...loggateway.Field) {
	l.warns = append(l.warns, msg)
}
func (l *estGateCaptureLogger) Error(string, ...loggateway.Field)          {}
func (l *estGateCaptureLogger) With(...loggateway.Field) loggateway.Logger { return l }

func resetEstProvider(t *testing.T) {
	t.Helper()
	SetPromptEstModeProvider(nil)
	estModeWarnOnce.Store(false)
	t.Cleanup(func() {
		SetPromptEstModeProvider(nil)
		estModeWarnOnce.Store(false)
	})
}

func estModeFixture() []trpcmodel.Message {
	return []trpcmodel.Message{{
		Role:    trpcmodel.RoleUser,
		Content: fixtureCJKProse + " " + fixtureLatinProse,
	}}
}

// TestPromptEstTokensGated_ModeMatrix 钉三档消费语义：
//   - 缺省（未注入）与显式 shadow：返回 legacy 值 + 恰一条 prompt.est_shadow
//     对照日志（零行为变化 + 影子证据双契约）；
//   - legacy：返回 legacy 值、零影子日志；
//   - segmented：返回 segmented 整片估算。sharedEstCal 无锚点接线，故与零
//     配置 SegmentedEstConfig{} 的纯函数值逐点相等。
func TestPromptEstTokensGated_ModeMatrix(t *testing.T) {
	resetEstProvider(t)
	msgs := estModeFixture()
	legacy := promptEstTokens(msgs)

	// 缺省 = shadow。
	lg := &estGateCaptureLogger{}
	if got := promptEstTokensGated(msgs, lg); got != legacy {
		t.Fatalf("default(shadow) gated = %d, want legacy %d", got, legacy)
	}
	if len(lg.infos) != 1 {
		t.Fatalf("default(shadow) must log exactly one comparison entry, got %d", len(lg.infos))
	}
	if step := lg.infos[0].fields["step_id"]; step != "prompt.est_shadow" {
		t.Fatalf("step_id=%v, want prompt.est_shadow", step)
	}
	if le, ok := lg.infos[0].fields["legacy_est"].(int64); !ok || int(le) != legacy {
		t.Fatalf("legacy_est=%v, want %d", lg.infos[0].fields["legacy_est"], legacy)
	}

	// 显式 shadow 同缺省。
	SetPromptEstModeProvider(func() string { return "shadow" })
	lgS := &estGateCaptureLogger{}
	if got := promptEstTokensGated(msgs, lgS); got != legacy || len(lgS.infos) != 1 {
		t.Fatalf("explicit shadow: got=%d infos=%d, want (%d, 1)", got, len(lgS.infos), legacy)
	}

	// legacy：零影子日志。
	SetPromptEstModeProvider(func() string { return "legacy" })
	lgL := &estGateCaptureLogger{}
	if got := promptEstTokensGated(msgs, lgL); got != legacy || len(lgL.infos) != 0 {
		t.Fatalf("legacy mode: got=%d infos=%d, want (%d, 0)", got, len(lgL.infos), legacy)
	}

	// segmented：与零配置分段纯函数逐点相等（sharedEstCal 零锚点钉）。
	SetPromptEstModeProvider(func() string { return "segmented" })
	lgSeg := &estGateCaptureLogger{}
	wantSeg := promptEstTokensSegmented(msgs, SegmentedEstConfig{})
	if got := promptEstTokensGated(msgs, lgSeg); got != wantSeg || len(lgSeg.infos) != 0 {
		t.Fatalf("segmented mode: got=%d infos=%d, want (%d, 0)", got, len(lgSeg.infos), wantSeg)
	}
}

// TestPromptEstTokensGated_BadModeFallsBackWithOnceWarn 钉坏档位行为：
// 消费值回落 shadow（安全侧，等同缺省），Warn 只落一次（防逐调用刷日志），
// 且影子对照照常产出（观测面不被坏配置打断）。
func TestPromptEstTokensGated_BadModeFallsBackWithOnceWarn(t *testing.T) {
	resetEstProvider(t)
	SetPromptEstModeProvider(func() string { return "wat" })
	msgs := estModeFixture()
	legacy := promptEstTokens(msgs)

	lg := &estGateCaptureLogger{}
	for i := 0; i < 3; i++ {
		if got := promptEstTokensGated(msgs, lg); got != legacy {
			t.Fatalf("call %d: bad mode must fall back to shadow value %d, got %d", i, legacy, got)
		}
	}
	if len(lg.warns) != 1 {
		t.Fatalf("bad mode must warn exactly once, got %d warns: %v", len(lg.warns), lg.warns)
	}
	if len(lg.infos) != 3 {
		t.Fatalf("shadow comparison must still land per call, got %d", len(lg.infos))
	}
}

// TestMessageEstTokensGated_ModesAndBounds 钉 message 级成对翻牌：
//   - legacy/shadow：恒等 messageEstTokens（shadow 档 message 级刻意不落影
//     子——P0-3① 冻结契约，函数签名上即无 lg 参）；
//   - segmented：逐消息 rune 类分段 ceil；
//   - 记档偏差界：Σ per-message ceil − 整片 ceil ∈ [0, N-1]（保守方向，
//     宁可多估不漏估）。
func TestMessageEstTokensGated_ModesAndBounds(t *testing.T) {
	resetEstProvider(t)
	msgs := []trpcmodel.Message{
		{Role: trpcmodel.RoleSystem, Content: fixtureLatinProse},
		{Role: trpcmodel.RoleUser, Content: fixtureCJKProse},
		{Role: trpcmodel.RoleAssistant, Content: fixtureCJKProse + " mid " + fixtureLatinProse},
	}

	for _, mode := range []string{"legacy", "shadow"} {
		SetPromptEstModeProvider(func() string { return mode })
		for i, m := range msgs {
			if got, want := messageEstTokensGated(m), messageEstTokens(m); got != want {
				t.Fatalf("mode=%s msg %d: gated=%d, want legacy primitive %d", mode, i, got, want)
			}
		}
	}

	SetPromptEstModeProvider(func() string { return "segmented" })
	sum := 0
	for i, m := range msgs {
		got := messageEstTokensGated(m)
		want := estimateTokensSegmentedCounts(runeClassCountsMessage(m), sharedEstCal.Config())
		if got != want {
			t.Fatalf("segmented msg %d: gated=%d, want segmented counts %d", i, got, want)
		}
		sum += got
	}
	whole := promptEstTokensGated(msgs, nil)
	drift := sum - whole
	if drift < 0 || drift > len(msgs)-1 {
		t.Fatalf("Σ per-msg ceil − whole ceil = %d, want ∈ [0, %d]（87 design 记档界）", drift, len(msgs)-1)
	}
}

// TestEstModeNilProviderResetsToShadow 钉 SetPromptEstModeProvider(nil) 的
// 复位语义：覆盖显式 legacy 后回 nil，消费面回到 shadow 缺省。
func TestEstModeNilProviderResetsToShadow(t *testing.T) {
	resetEstProvider(t)
	SetPromptEstModeProvider(func() string { return "legacy" })
	if got := currentEstMode(nil); got != estModeLegacy {
		t.Fatalf("explicit legacy: got %q", got)
	}
	SetPromptEstModeProvider(nil)
	if got := currentEstMode(nil); got != estModeShadow {
		t.Fatalf("nil provider must reset to shadow, got %q", got)
	}
}

// TestPrecheckModeProvider_Matrix 钉 D4-b durable 预检档位 provider：
// 缺省/nil/garbage 回落 observe，合法三档原样生效。
func TestPrecheckModeProvider_Matrix(t *testing.T) {
	SetDurablePrecheckModeProvider(nil)
	t.Cleanup(func() { SetDurablePrecheckModeProvider(nil) })

	if got := currentPrecheckMode(); got != precheckModeObserve {
		t.Fatalf("default must be observe, got %q", got)
	}
	for mode, want := range map[string]precheckMode{
		"off":      precheckModeOff,
		"observe":  precheckModeObserve,
		"on":       precheckModeOn,
		"garbage":  precheckModeObserve,
		"":         precheckModeObserve,
		"observee": precheckModeObserve,
	} {
		m := mode // capture for closure
		SetDurablePrecheckModeProvider(func() string { return m })
		if got := currentPrecheckMode(); got != want {
			t.Fatalf("provider %q: got %q, want %q", mode, got, want)
		}
	}
}
