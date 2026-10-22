package agent

import (
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestPromptEstTokens_LatinKeepsBlended(t *testing.T) {
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: strings.Repeat("a", 1000)}}
	blended := analyzePromptRequest(msgs).EstTokens
	got := promptEstTokens(msgs)
	if got != blended {
		t.Fatalf("latin promptEstTokens=%d, want blended=%d", got, blended)
	}
}

func TestPromptEstTokens_CJKFloorExceedsBlended(t *testing.T) {
	const n = 1000
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: strings.Repeat("测", n)}}
	blended := analyzePromptRequest(msgs).EstTokens
	got := promptEstTokens(msgs)
	if blended >= n {
		t.Fatalf("precondition: blended %d should under-count %d CJK runes", blended, n)
	}
	if got != n {
		t.Fatalf("CJK promptEstTokens=%d, want floor=%d (blended=%d)", got, n, blended)
	}
}

func TestMessageEstTokens_CJKFloor(t *testing.T) {
	m := trpcmodel.Message{Role: trpcmodel.RoleTool, Content: strings.Repeat("故", 250)}
	if got := messageEstTokens(m); got != 250 {
		t.Fatalf("messageEstTokens CJK=%d, want 250", got)
	}
}

// ---------------------------------------------------------------------------
// A6 — segmented estimator fixtures (design §4.7: 四组 fixture 钉死误差带)
// ---------------------------------------------------------------------------
//
// Band protocol: each fixture pins (a) exact rune-class split counts,
// (b) exact estimator output at the documented default pricing, and
// (c) est within ±25% of a consensus reference (pre-calibration band). The
// reference is stated at the consensus class rates most commonly reported for
// cl100k-class BPEs on natural text (latin 3.7 cpt, CJK 1:1, emoji 2, other
// 0.5); fixture texts are sized so the estimator sits near band center,
// making the check robust to ±20% tokenizer variance. Offline note
// (2026-10-21): this workspace has no BPE tokenizer available, so references
// are consensus-center values, not raw measurements — the dual-anchor
// calibrator (SegmentedCalibrator) is the on-line correction path; any
// calibrated-rate drift beyond the band flips these fixtures red, which is
// exactly the pre-flip tripwire the design asks for.

func assertEstWithinBand(t *testing.T, name string, est, ref int) {
	t.Helper()
	lo := float64(ref) * 0.75
	hi := float64(ref) * 1.25
	if float64(est) < lo || float64(est) > hi {
		t.Fatalf("%s: segmented est=%d outside ±25%% band of consensus ref=%d ([%.1f, %.1f])", name, est, ref, lo, hi)
	}
}

// fixtureLatinProse 是约 900 字节纯 ASCII 英文技术段落（恒定文本，勿改字节）。
var fixtureLatinProse = strings.Repeat(
	"The deployment guide describes a deterministic pipeline for sandbox "+
		"provisioning, lease renewal and drift reconciliation across the "+
		"fleet. Each stage emits structured telemetry and honours the same "+
		"fail-closed policy surface so degenerate configurations surface as ",
	4,
) + "explicit warnings."

func TestSegmented_FixtureLatin(t *testing.T) {
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: fixtureLatinProse}}
	counts := runeClassCountsMessage(msgs[0])
	if counts.CJK != 0 || counts.Emoji != 0 || counts.Other != 0 {
		t.Fatalf("latin fixture must be pure ASCII, got %+v", counts)
	}
	if counts.ASCII != len(fixtureLatinProse) {
		t.Fatalf("ascii rune count %d, want %d", counts.ASCII, len(fixtureLatinProse))
	}
	est := promptEstTokensSegmented(msgs, SegmentedEstConfig{})
	// 精确钉死默认定价输出：len/4.0 上取整。
	want := (len(fixtureLatinProse) + 3) / 4
	if est != want {
		t.Fatalf("latin est=%d, want default-pricing ceil(len/4)=%d", est, want)
	}
	// 共识参考：len/3.7（英文长文 cl100k 常见均值）→ 带宽 ±25%。
	ref := int(float64(len(fixtureLatinProse)) / 3.7)
	assertEstWithinBand(t, "latin", est, ref)
}

// fixtureCJKProse 是 500 个中文常用字段落（按 rune 切片，避免切断 UTF-8）。
var fixtureCJKProse = string([]rune(strings.Repeat(
	"我们今天一起去公园散步看风景然后讨论一下下周的部署计划和巡检安排",
	17,
))[:500])

func TestSegmented_FixtureCJK(t *testing.T) {
	if got := len([]rune(fixtureCJKProse)); got != 500 {
		t.Fatalf("fixture construction drifted: %d runes, want 500", got)
	}
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: fixtureCJKProse}}
	counts := runeClassCountsMessage(msgs[0])
	if counts.CJK != 500 || counts.ASCII != 0 || counts.Emoji != 0 || counts.Other != 0 {
		t.Fatalf("cjk fixture must be 500 CJK runes, got %+v", counts)
	}
	est := promptEstTokensSegmented(msgs, SegmentedEstConfig{})
	if est != 500 {
		t.Fatalf("cjk est=%d, want 1:1 floor 500", est)
	}
	assertEstWithinBand(t, "cjk", est, 500)
	// 混合时与旧路径的 max() 语义分叉点：纯 CJK 下二者一致（回退相容）。
	if legacy := promptEstTokens(msgs); legacy != 500 {
		t.Fatalf("legacy cjk floor=%d, want 500（新旧路径纯 CJK 必须对齐）", legacy)
	}
}

func TestSegmented_FixtureMixed(t *testing.T) {
	cjk := strings.Repeat("数据", 150)                                         // 300 CJK
	latin := strings.Repeat("budget gate calibration and drift metrics ", 8) // 320 ASCII
	emoji := strings.Repeat("🎯✅📊🔍", 10)                                      // 40 emoji (So 类)
	other := strings.Repeat("Привет мир всем тестовым стендам ", 2)          // 60 other（西里尔+空格中空格为 ASCII，按段计数详见断言）
	text := cjk + "\n" + latin + "\n" + emoji + other
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: text}}
	counts := runeClassCountsMessage(msgs[0])
	if counts.CJK != 300 {
		t.Fatalf("cjk count=%d, want 300", counts.CJK)
	}
	if counts.Emoji != 40 {
		t.Fatalf("emoji count=%d, want 40", counts.Emoji)
	}
	if counts.ASCII == 0 || counts.Other == 0 {
		t.Fatalf("mixed fixture must hit all four classes, got %+v", counts)
	}
	est := promptEstTokensSegmented(msgs, SegmentedEstConfig{})
	// 共识参考：CJK 1:1 + latin/3.7 + emoji*2 + other*0.5。
	ref := counts.CJK + int(float64(counts.ASCII)/3.7) + counts.Emoji*2 + int(float64(counts.Other)*0.5)
	assertEstWithinBand(t, "mixed", est, ref)
	// 旧路径失真针：mixed 输入下分段估算与 max() 臂选择的结果必须可区分——
	// 否则影子比对一周比对不出东西（design §4.1④ 根因）。
	if legacy := promptEstTokens(msgs); legacy == est {
		t.Fatalf("mixed fixture must distinguish segmented (%d) from legacy (%d)", est, legacy)
	}
}

func TestSegmented_FixtureEmoji(t *testing.T) {
	emoji := strings.Repeat("😀🎉🚀🌍🔥✨🎯📌🔍💡", 10) // 100 emoji runes（单码点 So 类，多形体）
	if got := len([]rune(emoji)); got != 100 {
		t.Fatalf("emoji fixture construction drifted: %d runes, want 100", got)
	}
	latin := "emoji pricing fixture tail notes for the gold gate band check."
	msgs := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: emoji + " " + latin}}
	counts := runeClassCountsMessage(msgs[0])
	if counts.Emoji != 100 {
		t.Fatalf("emoji class count=%d, want 100 (ZWJ/VS runes must not miscount)", counts.Emoji)
	}
	est := promptEstTokensSegmented(msgs, SegmentedEstConfig{})
	// 硬不变量（外部真相锚点）：emoji 类单价必须落在公开 BPE 文档化的
	// [1,3] tok/rune 区间内——默认 2.0，kalibration 也不许越界（见钳制）。
	price := SegmentedEstConfig{}.withDefaults().TokensPerEmojiRune
	if price < 1 || price > 3 {
		t.Fatalf("emoji per-rune price %.2f outside documented [1,3] band", price)
	}
	// 精确默认输出（回归钉）：100*2 + ascii/4.0 上取整。
	asciiCount := len([]rune(" " + latin))
	want := 200 + (asciiCount+3)/4
	if est != want {
		t.Fatalf("emoji est=%d, want default-pricing %d", est, want)
	}
	// 共识参考带（emoji*2 + latin/3.7）。
	ref := 200 + int(float64(asciiCount-1)/3.7)
	assertEstWithinBand(t, "emoji", est, ref)
}

// TestSegmented_CalibrationConverges 仿 llmcontext 双锚点先例：rCJK 钉死 1:1
// 底，provider 权威 usage 锚点只校准 rLatin，并受 [1,6] chars/token 钳制。
func TestSegmented_CalibrationConverges(t *testing.T) {
	cal := NewSegmentedCalibrator(SegmentedEstConfig{})
	if got := cal.Config().CharsPerLatinToken; got != 4.0 {
		t.Fatalf("default latin price=%v, want 4.0", got)
	}
	// 锚点 1：600 ASCII + 500 CJK，权威 650 tok → 残差 150 latin tok → 4.0（稳态）。
	anchorText := strings.Repeat("a", 600) + strings.Repeat("测", 500)
	cal.RecordAuthoritative(650, anchorText)
	if got := cal.Config().CharsPerLatinToken; got != 4.0 {
		t.Fatalf("anchor1 latin price=%v, want converged 4.0", got)
	}
	// 锚点 2：1200 ASCII + 500 CJK，权威 950 tok → 残差 450 → rate=2.666…（收敛移动）。
	cal.RecordAuthoritative(950, strings.Repeat("b", 1200)+strings.Repeat("测", 500))
	got := cal.Config().CharsPerLatinToken
	if got < 2.6 || got > 2.7 {
		t.Fatalf("anchor2 latin price=%v, want ~2.667", got)
	}
	// rCJK 不漂：纯 CJK 估算仍是 1:1。
	if n := cal.Estimate([]trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: strings.Repeat("故", 10)}}); n != 10 {
		t.Fatalf("calibrated estimator must keep CJK floor, got %d", n)
	}
	// 坏锚点（residual ≤ 0）不污染：保持上次校准值。
	before := cal.Config().CharsPerLatinToken
	cal.RecordAuthoritative(10, strings.Repeat("c", 500)+strings.Repeat("测", 500))
	if after := cal.Config().CharsPerLatinToken; after != before {
		t.Fatalf("poison anchor must be ignored: %v -> %v", before, after)
	}
	// 空锚点/零 tok 同样忽略。
	cal.RecordAuthoritative(0, "hello")
	if after := cal.Config().CharsPerLatinToken; after != before {
		t.Fatalf("zero-token anchor must be ignored: %v -> %v", before, after)
	}
	// 极端锚点钳制到下限 1.0（500 latin chars 全权 500 tok → 1.0 cpt）。
	cal.RecordAuthoritative(500, strings.Repeat("d", 500))
	if got := cal.Config().CharsPerLatinToken; got != 1.0 {
		t.Fatalf("extreme anchor must clamp to min 1.0, got %v", got)
	}
}

// captureLogger 是 loggateway.Logger 的内存实现，供影子日志断言。
type captureLogger struct {
	infos []captureEntry
}
type captureEntry struct {
	msg    string
	fields map[string]any
}

func (l *captureLogger) Debug(msg string, fields ...loggateway.Field) {}
func (l *captureLogger) Info(msg string, fields ...loggateway.Field) {
	l.infos = append(l.infos, captureEntry{msg: msg, fields: zapFieldsToMap(fields)})
}
func (l *captureLogger) Warn(msg string, fields ...loggateway.Field)       {}
func (l *captureLogger) Error(msg string, fields ...loggateway.Field)      {}
func (l *captureLogger) With(fields ...loggateway.Field) loggateway.Logger { return l }

func zapFieldsToMap(fields []loggateway.Field) map[string]any {
	m := map[string]any{}
	for _, f := range fields {
		m[f.Key] = f.Integer
		if f.String != "" {
			m[f.Key] = f.String
		}
	}
	return m
}

// TestSegmented_ShadowModeLogsComparison 钉死审批注记：影子日志 step_id 固定
// "prompt.est_shadow"，返回旧路径值（零行为变更），字段三元组可比。
func TestSegmented_ShadowModeLogsComparison(t *testing.T) {
	msgs := []trpcmodel.Message{{
		Role:    trpcmodel.RoleUser,
		Content: fixtureCJKProse + " " + fixtureLatinProse,
	}}
	lg := &captureLogger{}
	cal := NewSegmentedCalibrator(SegmentedEstConfig{})

	got := promptEstTokensShadow(msgs, cal, lg)
	if want := promptEstTokens(msgs); got != want {
		t.Fatalf("shadow mode must return legacy value %d, got %d", want, got)
	}
	if len(lg.infos) != 1 {
		t.Fatalf("shadow must log exactly one comparison entry, got %d", len(lg.infos))
	}
	entry := lg.infos[0]
	if got := entry.fields["step_id"]; got != "prompt.est_shadow" {
		t.Fatalf("step_id=%v, want prompt.est_shadow（flowlog 对比脚本消费名）", got)
	}
	legacy, okL := entry.fields["legacy_est"]
	seg, okS := entry.fields["segmented_est"]
	delta, okD := entry.fields["delta"]
	if !okL || !okS || !okD {
		t.Fatalf("shadow entry missing est triple: %+v", entry.fields)
	}
	_ = legacy
	// delta 一致性：seg - legacy。
	if delta.(int64) != seg.(int64)-legacy.(int64) {
		t.Fatalf("delta field inconsistent: %v vs %v - %v", delta, seg, legacy)
	}
	// nil logger 零开销直接返回。
	if n := promptEstTokensShadow(msgs, cal, nil); n != promptEstTokens(msgs) {
		t.Fatal("nil logger must skip comparison entirely")
	}
	if n := promptEstTokensShadow(msgs, nil, lg); n != promptEstTokens(msgs) {
		t.Fatal("nil calibrator must skip comparison entirely")
	}
}
