package biz

import (
	"strings"
	"testing"
)

// TestSensitiveScanDoesNotLeakSecret ensures hit descriptors never carry the
// matched secret material itself (report safety).
func TestSensitiveScanDoesNotLeakSecret(t *testing.T) {
	secret := "sk-leakcheck1234567890abcd"
	hits := DetectSensitiveContent("+k := \"" + secret + "\"\n")
	if len(hits) == 0 {
		t.Fatal("expected hit")
	}
	for _, h := range hits {
		if h == secret {
			t.Fatalf("hit descriptor leaked secret material: %q", h)
		}
	}
}

func TestPlanSIVerification_KindAwareGates(t *testing.T) {
	goChange := []PatchFileChange{{Path: "internal/biz/foo.go", Kind: PatchChangeModified}}
	cfgChange := []PatchFileChange{{Path: "configs/config.yaml", Kind: PatchChangeModified}}
	webChange := []PatchFileChange{{Path: "web/src/App.vue", Kind: PatchChangeModified}}

	codeGo := PlanSIVerification(PatchKindCode, goChange)
	if !codeGo.ShouldRun(SandboxGateBuild) || !codeGo.ShouldRun(SandboxGateTest) || !codeGo.ShouldRun(SandboxGateLint) {
		t.Fatalf("code+go should run G1-G3, plan=%+v", codeGo)
	}
	if codeGo.ShouldRun(SandboxGateWebLint) {
		t.Error("code+go must not schedule web lint")
	}

	cfg := PlanSIVerification(PatchKindConfig, cfgChange)
	if cfg.ShouldRun(SandboxGateBuild) || cfg.ShouldRun(SandboxGateTest) || cfg.ShouldRun(SandboxGateLint) {
		t.Fatalf("config-only must skip G1-G3, plan=%+v", cfg)
	}
	if !strings.Contains(cfg.SkipReason(SandboxGateTest), "not required") {
		t.Errorf("config G2 skip reason = %q", cfg.SkipReason(SandboxGateTest))
	}

	cfgWithGo := PlanSIVerification(PatchKindConfig, goChange)
	if !cfgWithGo.ShouldRun(SandboxGateBuild) || cfgWithGo.ShouldRun(SandboxGateTest) || !cfgWithGo.ShouldRun(SandboxGateLint) {
		t.Fatalf("config+go runs G1/G3 but not G2, plan=%+v", cfgWithGo)
	}

	web := PlanSIVerification(PatchKindCode, webChange)
	if web.ShouldRun(SandboxGateBuild) || web.ShouldRun(SandboxGateTest) {
		t.Fatalf("web-only must not run go test/build, plan=%+v", web)
	}
	if !web.ShouldRun(SandboxGateWebLint) {
		t.Error("web-only must schedule web lint")
	}
}

func TestParseGoTestFailures(t *testing.T) {
	names, setup := ParseGoTestFailures("--- FAIL: TestFoo (0.01s)\n--- FAIL: TestBar/sub (0.00s)\nFAIL\tpkg\t0.02s\n")
	if setup || len(names) != 2 || names[0] != "TestFoo" || names[1] != "TestBar/sub" {
		t.Fatalf("names=%v setup=%v", names, setup)
	}
	_, setup = ParseGoTestFailures("# pkg [pkg.test]\nFAIL\tpkg [setup failed]\n")
	if !setup {
		t.Fatal("setup failed must be detected")
	}
}

func TestApplyKnownFailExemption(t *testing.T) {
	failed := SandboxGateResult{
		Gate: SandboxGateTest, Passed: false,
		Output: "--- FAIL: TestOld (0.01s)\nFAIL\tpkg\t0.02s\n",
	}
	got := ApplyKnownFailExemption(failed, []string{"TestOld"})
	if !got.Passed || !strings.Contains(got.Output, "exempted HEAD-known") {
		t.Fatalf("same HEAD failure must be exempted: %+v", got)
	}
	mixed := failed
	mixed.Output = "--- FAIL: TestOld (0.01s)\n--- FAIL: TestNew (0.00s)\n"
	got = ApplyKnownFailExemption(mixed, []string{"TestOld"})
	if got.Passed {
		t.Fatal("new failure must not be exempted")
	}
	compile := SandboxGateResult{
		Gate: SandboxGateTest, Passed: false,
		Output: "FAIL\tpkg [setup failed]\n",
	}
	if ApplyKnownFailExemption(compile, []string{"TestOld"}).Passed {
		t.Fatal("setup/compile failure must not be exempted")
	}
}
