package biz

import (
	"strings"
	"sync"
	"testing"

	skillpkg "aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/loggateway"
)

// TestParseSkillTrustLevel 钉死解析面与 fail-closed（未知值 ok=false）。
func TestParseSkillTrustLevel(t *testing.T) {
	cases := []struct {
		raw  string
		want SkillTrustLevel
		ok   bool
	}{
		{"T1", SkillTrustT1Builtin, true},
		{"t2", SkillTrustT2Audited, true},
		{" T3 ", SkillTrustT3Standard, true},
		{"T4", SkillTrustT4Untrusted, true},
		{"", "", false},
		{"T0", "", false},
		{"t5", "", false},
		{"verified", "", false},
	}
	for _, c := range cases {
		got, ok := ParseSkillTrustLevel(c.raw)
		if got != c.want || ok != c.ok {
			t.Fatalf("raw=%q: expected (%q,%v), got (%q,%v)", c.raw, c.want, c.ok, got, ok)
		}
	}
}

// TestParseSkillSource 钉死来源解析与大小写归一。
func TestParseSkillSource(t *testing.T) {
	cases := []struct {
		raw  string
		want SkillSource
		ok   bool
	}{
		{"builtin", SkillSourceBuiltin, true},
		{"LLM_Evolution", SkillSourceLLMEvolution, true},
		{"zip_import", SkillSourceZipImport, true},
		{"user_created", SkillSourceUserCreated, true},
		{"", "", false},
		{"random", "", false},
	}
	for _, c := range cases {
		got, ok := ParseSkillSource(c.raw)
		if got != c.want || ok != c.ok {
			t.Fatalf("raw=%q: expected (%q,%v), got (%q,%v)", c.raw, c.want, c.ok, got, ok)
		}
	}
}

// TestTrustRank_Ordering 钉死排序语义：T1 最高、T4 最低、未知=0。
func TestTrustRank_Ordering(t *testing.T) {
	if !(TrustRank(SkillTrustT1Builtin) > TrustRank(SkillTrustT2Audited) &&
		TrustRank(SkillTrustT2Audited) > TrustRank(SkillTrustT3Standard) &&
		TrustRank(SkillTrustT3Standard) > TrustRank(SkillTrustT4Untrusted) &&
		TrustRank(SkillTrustT4Untrusted) > TrustRank("")) {
		t.Fatalf("bad rank ordering: %d %d %d %d %d",
			TrustRank(SkillTrustT1Builtin), TrustRank(SkillTrustT2Audited),
			TrustRank(SkillTrustT3Standard), TrustRank(SkillTrustT4Untrusted), TrustRank(""))
	}
}

// TestDefaultTrustForSource 钉死默认降级策略：
// builtin→T1、zip_import→T4、llm_evolution/user_created/未知→T3（存量兼容）。
func TestDefaultTrustForSource(t *testing.T) {
	cases := []struct {
		source SkillSource
		want   SkillTrustLevel
	}{
		{SkillSourceBuiltin, SkillTrustT1Builtin},
		{SkillSourceZipImport, SkillTrustT4Untrusted},
		{SkillSourceLLMEvolution, SkillTrustT3Standard},
		{SkillSourceUserCreated, SkillTrustT3Standard},
		{SkillSource(""), SkillTrustT3Standard}, // 存量默认 T3 可加载
	}
	for _, c := range cases {
		if got := DefaultTrustForSource(c.source); got != c.want {
			t.Fatalf("source=%q: expected %q, got %q", c.source, c.want, got)
		}
	}
}

// TestResolveSkillTrust_StricterWins 钉死 fail-closed 取严：
// 声明档比来源默认更严取声明；声明档宽松于来源默认仍取来源默认；
// 声明无法解析视同未声明。
func TestResolveSkillTrust_StricterWins(t *testing.T) {
	cases := []struct {
		name     string
		source   SkillSource
		declared string
		want     SkillTrustLevel
	}{
		{"声明更严取声明", SkillSourceBuiltin, "t4", SkillTrustT4Untrusted},
		{"声明宽松不放宽", SkillSourceZipImport, "t1", SkillTrustT4Untrusted},
		{"声明非法按默认", SkillSourceBuiltin, "bogus", SkillTrustT1Builtin},
		{"空声明按默认", SkillSourceUserCreated, "", SkillTrustT3Standard},
		{"未知来源默认", SkillSource(""), "", SkillTrustT3Standard},
		{"未知来源+声明T4", SkillSource(""), "T4", SkillTrustT4Untrusted},
	}
	for _, c := range cases {
		if got := ResolveSkillTrust(c.source, c.declared); got != c.want {
			t.Fatalf("%s: source=%q declared=%q: expected %q, got %q", c.name, c.source, c.declared, c.want, got)
		}
	}
}

// TestRequiredTrustForLayer 钉死加载层门槛：L1 全档/T3+ 指令/T2+ 脚本。
func TestRequiredTrustForLayer(t *testing.T) {
	cases := []struct {
		layer SkillLoadLayer
		want  SkillTrustLevel
	}{
		{SkillLoadLayerL1Metadata, SkillTrustT4Untrusted},
		{SkillLoadLayerL2Instruction, SkillTrustT3Standard},
		{SkillLoadLayerL3Script, SkillTrustT2Audited},
		{SkillLoadLayer("L9"), SkillTrustT1Builtin}, // 未知层 fail-closed 最严
	}
	for _, c := range cases {
		if got := RequiredTrustForLayer(c.layer); got != c.want {
			t.Fatalf("layer=%q: expected %q, got %q", c.layer, c.want, got)
		}
	}
}

// TestToolSurfaceForTrust 钉死 TrustLevel→ToolsProfile 工具面映射矩阵。
func TestToolSurfaceForTrust(t *testing.T) {
	cases := []struct {
		level SkillTrustLevel
		want  SkillToolSurface
	}{
		{SkillTrustT1Builtin, SkillToolSurface{MetadataVisible: true, InstructionInject: true, ScriptExecute: true}},
		{SkillTrustT2Audited, SkillToolSurface{MetadataVisible: true, InstructionInject: true, ScriptExecute: true}},
		{SkillTrustT3Standard, SkillToolSurface{MetadataVisible: true, InstructionInject: true, ScriptExecute: false}},
		{SkillTrustT4Untrusted, SkillToolSurface{MetadataVisible: true, InstructionInject: false, ScriptExecute: false}},
		{SkillTrustLevel(""), SkillToolSurface{MetadataVisible: false, InstructionInject: false, ScriptExecute: false}}, // fail-closed
	}
	for _, c := range cases {
		if got := ToolSurfaceForTrust(c.level); got != c.want {
			t.Fatalf("level=%q: expected %+v, got %+v", c.level, c.want, got)
		}
	}
}

// TestEvolutionLandingPolicy 钉死 LLM 产物落户规则：
// 最高 validated、任意来源不自动 applied（§8.4⑥）。
func TestEvolutionLandingPolicy(t *testing.T) {
	if got := LLMEvolutionLandingMaxState(); got != skillpkg.SkillStateValidated {
		t.Fatalf("expected max landing state validated, got %q", got)
	}
	for _, src := range []SkillSource{SkillSourceBuiltin, SkillSourceLLMEvolution, SkillSourceZipImport, SkillSourceUserCreated, SkillSource("")} {
		if EvolutionAutoApplyAllowed(src) {
			t.Fatalf("source=%q: evolution auto-apply must never be allowed", src)
		}
	}
}

// TestNormalizeTrustGateMode 钉死三档归一：on 本轮降级为 shadow（永不真拒），
// 未知值降级 off，两者均 degraded=true。
func TestNormalizeTrustGateMode(t *testing.T) {
	cases := []struct {
		raw      string
		mode     TrustGateMode
		degraded bool
	}{
		{"", TrustGateModeOff, false},
		{"off", TrustGateModeOff, false},
		{"shadow", TrustGateModeShadow, false},
		{"on", TrustGateModeShadow, true},
		{"Shadow", TrustGateModeShadow, false},
		{"garbage", TrustGateModeOff, true},
	}
	for _, c := range cases {
		mode, degraded := NormalizeTrustGateMode(c.raw)
		if mode != c.mode || degraded != c.degraded {
			t.Fatalf("raw=%q: expected (%q,%v), got (%q,%v)", c.raw, c.mode, c.degraded, mode, degraded)
		}
	}
}

// TestEvaluateTrustGate 钉死 shadow 门禁纯判定：
// resolved 低于 required 才 wouldBlock；存量默认（空来源空声明）在 L2 门槛下永不误拒。
func TestEvaluateTrustGate(t *testing.T) {
	l2 := RequiredTrustForLayer(SkillLoadLayerL2Instruction)

	// 存量默认 T3 可加载：空来源空声明 must NOT block（§8.5① 防误挂全拒载）。
	if resolved, wouldBlock := EvaluateTrustGate("", "", l2); wouldBlock || resolved != SkillTrustT3Standard {
		t.Fatalf("legacy default must load at L2: resolved=%q wouldBlock=%v", resolved, wouldBlock)
	}
	// LLM 产物默认 T3：L2 不拒、L3 拒。
	if _, wouldBlock := EvaluateTrustGate("llm_evolution", "", l2); wouldBlock {
		t.Fatal("llm_evolution default T3 must pass L2")
	}
	if _, wouldBlock := EvaluateTrustGate("llm_evolution", "", RequiredTrustForLayer(SkillLoadLayerL3Script)); !wouldBlock {
		t.Fatal("llm_evolution default T3 must be blocked at L3")
	}
	// zip 导入默认 T4：L2 拒。
	if _, wouldBlock := EvaluateTrustGate("zip_import", "", l2); !wouldBlock {
		t.Fatal("zip_import default T4 must be blocked at L2")
	}
	// 声明 T1 无法放宽 zip 默认 T4（取严）。
	if resolved, wouldBlock := EvaluateTrustGate("zip_import", "T1", l2); !wouldBlock || resolved != SkillTrustT4Untrusted {
		t.Fatalf("declared T1 must not loosen zip T4: resolved=%q wouldBlock=%v", resolved, wouldBlock)
	}
}

// captureLogger 捕获 loggateway 结构化日志供测试断言。
type captureLogger struct {
	mu     sync.Mutex
	fields []loggateway.Field
	msg    []string
	byLvl  map[string]int
}

func newCaptureLogger() *captureLogger { return &captureLogger{byLvl: map[string]int{}} }

func (c *captureLogger) record(lvl, msg string, fields []loggateway.Field) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byLvl[lvl]++
	c.msg = append(c.msg, msg)
	c.fields = append(c.fields, fields...)
}
func (c *captureLogger) Debug(msg string, f ...loggateway.Field) { c.record("debug", msg, f) }
func (c *captureLogger) Info(msg string, f ...loggateway.Field)  { c.record("info", msg, f) }
func (c *captureLogger) Warn(msg string, f ...loggateway.Field)  { c.record("warn", msg, f) }
func (c *captureLogger) Error(msg string, f ...loggateway.Field) { c.record("error", msg, f) }
func (c *captureLogger) With(f ...loggateway.Field) loggateway.Logger {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fields = append([]loggateway.Field{}, append(c.fields, f...)...)
	return c
}

// TestNewTrustGateHook_ShadowNeverFilters 钉死 shadow 门禁核心语义：
// would_block 计数结构化日志输出、永不真拒（on 输入降级 shadow 也不拒）。
func TestNewTrustGateHook_ShadowNeverFilters(t *testing.T) {
	lg := newCaptureLogger()
	hook := NewTrustGateHook(lg, TrustGateModeOn, RequiredTrustForLayer(SkillLoadLayerL2Instruction))
	if hook == nil {
		t.Fatal("on mode must degrade to shadow hook, got nil")
	}

	candidates := []skillpkg.RuntimeCandidate{
		{SkillID: "skill_1", Slug: "legacy", Source: "", TrustLevel: ""},             // 存量 T3 不拒
		{SkillID: "skill_2", Slug: "ext", Source: "zip_import", TrustLevel: ""},      // T4 拒
		{SkillID: "skill_3", Slug: "llm", Source: "llm_evolution", TrustLevel: "T3"}, // T3 不拒
		{SkillID: "skill_4", Slug: "builtin", Source: "builtin", TrustLevel: ""},     // T1 不拒
	}
	for _, c := range candidates {
		hook(c)
	}

	// 模式降级告警 + 恰好 1 条 would_block Warn（skill_2）。
	if lg.byLvl["warn"] != 2 {
		t.Fatalf("expected 2 warn logs (mode degraded + 1 would_block), got=%d; msgs=%v", lg.byLvl["warn"], lg.msg)
	}
	// hook 无返回值——构造上就不可能真拒（过滤逻辑不存在于求值面）。
	var sawBlocked bool
	for _, f := range lg.fields {
		if f.Key == "would_block" {
			sawBlocked = true
		}
	}
	if !sawBlocked {
		t.Fatal("expected would_block structured field in logs")
	}
	// required_level 字段为 L2 门槛 T3。
	var foundRequired bool
	for _, f := range lg.fields {
		if f.Key == "required_level" && f.String == string(SkillTrustT3Standard) {
			foundRequired = true
		}
	}
	if !foundRequired {
		t.Fatal("expected required_level=T3 structured field")
	}
}

// TestNewTrustGateHook_OffIsNil 钉死 OFF=零路径。
func TestNewTrustGateHook_OffIsNil(t *testing.T) {
	if hook := NewTrustGateHook(loggateway.NewNoop(), TrustGateModeOff, SkillTrustT3Standard); hook != nil {
		t.Fatal("off mode must return nil hook (zero behavior change)")
	}
}

// TestNewTrustGateHook_PassSilent 钉死通过候选零日志（估算影子先例：免费路径不打日志）。
func TestNewTrustGateHook_PassSilent(t *testing.T) {
	lg := newCaptureLogger()
	hook := NewTrustGateHook(lg, TrustGateModeShadow, RequiredTrustForLayer(SkillLoadLayerL2Instruction))
	hook(skillpkg.RuntimeCandidate{SkillID: "s1", Source: "builtin"})
	if lg.byLvl["warn"] != 0 {
		t.Fatalf("passing candidate must be silent, got=%d warn", lg.byLvl["warn"])
	}
}

// TestSkillEventTrustConstants 钉死事件名与用途注释一致（防漂移）。
func TestSkillEventTrustConstants(t *testing.T) {
	if SkillEventTrustChanged != "skill.trust_changed" || SkillEventTrustFiltered != "skill.trust_filtered" {
		t.Fatalf("unexpected trust event names: %q %q", SkillEventTrustChanged, SkillEventTrustFiltered)
	}
	if !strings.HasPrefix(SkillEventTrustFiltered, "skill.") {
		t.Fatal("trust_filtered must be namespaced under skill.")
	}
}

// TestDefaultLegacySkillTrust 钉死存量默认信任包。
func TestDefaultLegacySkillTrust(t *testing.T) {
	got := DefaultLegacySkillTrust()
	if got.Level != SkillTrustT3Standard {
		t.Fatalf("expected legacy default T3, got %q", got.Level)
	}
}
