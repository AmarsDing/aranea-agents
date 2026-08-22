package biz

import (
	"context"
	"strings"
	"testing"
)

// ── T2.1: RepoSandbox port (design §4.2) ─────────────────────────────────────

// fakeRepoSandbox is a test double proving the RepoSandbox port is
// implementable and capturing call arguments for behavior assertions.
type fakeRepoSandbox struct {
	preparePath    string
	prepareErr     error
	appliedDiff    string
	appliedPath    string
	applyErr       error
	gateResults    map[SandboxGateKind]SandboxGateResult
	gateErr        error
	lastGatePkgs   []string
	cleanupInvoked bool
}

func (f *fakeRepoSandbox) PrepareWorktree(ctx context.Context, runID, baseRef string) (string, func(), error) {
	if f.prepareErr != nil {
		return "", nil, f.prepareErr
	}
	return f.preparePath, func() { f.cleanupInvoked = true }, nil
}

func (f *fakeRepoSandbox) ApplyDiff(ctx context.Context, path, diff string) error {
	f.appliedPath = path
	f.appliedDiff = diff
	return f.applyErr
}

func (f *fakeRepoSandbox) ResetWorktree(_ context.Context, _ string) error { return nil }

func (f *fakeRepoSandbox) ProbeTestFailures(context.Context, string, []string) ([]string, error) {
	return nil, nil
}

func (f *fakeRepoSandbox) RunGate(ctx context.Context, path string, gate SandboxGateKind, pkgs []string) (SandboxGateResult, error) {
	f.lastGatePkgs = pkgs
	if f.gateErr != nil {
		return SandboxGateResult{}, f.gateErr
	}
	if r, ok := f.gateResults[gate]; ok {
		return r, nil
	}
	return SandboxGateResult{Gate: gate, Passed: true}, nil
}

// Compile-time assertion: the fake satisfies the port.
var _ RepoSandbox = (*fakeRepoSandbox)(nil)

func TestRepoSandbox_PortContract(t *testing.T) {
	fake := &fakeRepoSandbox{
		preparePath: t.TempDir(),
		gateResults: map[SandboxGateKind]SandboxGateResult{
			SandboxGateBuild: {Gate: SandboxGateBuild, Passed: true, Output: "ok", DurationMS: 12},
		},
	}

	path, cleanup, err := fake.PrepareWorktree(context.Background(), "run-1", "HEAD")
	if err != nil {
		t.Fatalf("PrepareWorktree: %v", err)
	}
	if path == "" {
		t.Fatal("PrepareWorktree returned empty path")
	}
	if cleanup == nil {
		t.Fatal("PrepareWorktree returned nil cleanup")
	}

	if err := fake.ApplyDiff(context.Background(), path, "diff --git a/x.go b/x.go"); err != nil {
		t.Fatalf("ApplyDiff: %v", err)
	}
	if fake.appliedPath != path {
		t.Errorf("ApplyDiff path = %q, want %q", fake.appliedPath, path)
	}

	res, err := fake.RunGate(context.Background(), path, SandboxGateBuild, []string{"./internal/biz/..."})
	if err != nil {
		t.Fatalf("RunGate: %v", err)
	}
	if !res.Passed || res.Gate != SandboxGateBuild {
		t.Errorf("RunGate result = %+v, want passed g1_build", res)
	}
	if len(fake.lastGatePkgs) != 1 || fake.lastGatePkgs[0] != "./internal/biz/..." {
		t.Errorf("RunGate pkgs = %v", fake.lastGatePkgs)
	}

	cleanup()
	if !fake.cleanupInvoked {
		t.Error("cleanup func was not invoked")
	}
}

// TestSandboxGateKinds_Distinct ensures gate kind values stay unique and
// stable (they are persisted into verification_report JSON).
func TestSandboxGateKinds_Distinct(t *testing.T) {
	kinds := []SandboxGateKind{
		SandboxGateBuild, SandboxGateTest, SandboxGateLint,
		SandboxGateWebLint, SandboxGateCritic, SandboxGateEvalBase,
	}
	seen := map[SandboxGateKind]bool{}
	for _, k := range kinds {
		if k == "" {
			t.Fatal("empty gate kind")
		}
		if seen[k] {
			t.Errorf("duplicate gate kind %q", k)
		}
		seen[k] = true
	}
}

// ── T2.3: affected scope derivation (design D4 G2) ───────────────────────────

func TestDeriveAffectedScopes(t *testing.T) {
	cases := []struct {
		name       string
		changes    []PatchFileChange
		wantGoPkgs []string
		wantWeb    bool
	}{
		{
			name:       "biz go file maps to package tree",
			changes:    []PatchFileChange{{Path: "internal/biz/foo.go", Kind: PatchChangeModified}},
			wantGoPkgs: []string{"./internal/biz/..."},
		},
		{
			name:       "nested go dir keeps full dir",
			changes:    []PatchFileChange{{Path: "internal/data/ent/schema/foo.go", Kind: PatchChangeModified}},
			wantGoPkgs: []string{"./internal/data/ent/schema/..."},
		},
		{
			name:       "root-level go file",
			changes:    []PatchFileChange{{Path: "main.go", Kind: PatchChangeAdded}},
			wantGoPkgs: []string{"./"},
		},
		{
			name:    "web file flips web scope only",
			changes: []PatchFileChange{{Path: "web/src/features/x/y.ts", Kind: PatchChangeModified}},
			wantWeb: true,
		},
		{
			name:    "docs file affects nothing",
			changes: []PatchFileChange{{Path: "docs/development/73-x.md", Kind: PatchChangeModified}},
		},
		{
			name: "mixed go + web + docs deduped and sorted",
			changes: []PatchFileChange{
				{Path: "internal/service/chat.go", Kind: PatchChangeModified},
				{Path: "web/src/a.ts", Kind: PatchChangeModified},
				{Path: "internal/service/chat_test.go", Kind: PatchChangeModified},
				{Path: "internal/biz/x.go", Kind: PatchChangeModified},
				{Path: "README.md", Kind: PatchChangeModified},
			},
			wantGoPkgs: []string{"./internal/biz/...", "./internal/service/..."},
			wantWeb:    true,
		},
		{
			name: "deleted go file still scopes its package",
			changes: []PatchFileChange{
				{Path: "internal/agent/old.go", Kind: PatchChangeDeleted},
			},
			wantGoPkgs: []string{"./internal/agent/..."},
		},
		{
			name:    "empty diff",
			changes: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			goPkgs, web := DeriveAffectedScopes(tc.changes)
			if web != tc.wantWeb {
				t.Errorf("web scope = %v, want %v", web, tc.wantWeb)
			}
			if len(goPkgs) != len(tc.wantGoPkgs) {
				t.Fatalf("goPkgs = %v, want %v", goPkgs, tc.wantGoPkgs)
			}
			for i := range goPkgs {
				if goPkgs[i] != tc.wantGoPkgs[i] {
					t.Fatalf("goPkgs = %v, want %v", goPkgs, tc.wantGoPkgs)
				}
			}
		})
	}
}

// ── T2.3 helper: unified diff file parsing ───────────────────────────────────

func TestParseUnifiedDiffFiles(t *testing.T) {
	cases := []struct {
		name string
		diff string
		want []PatchFileChange
	}{
		{
			name: "modified single file",
			diff: "diff --git a/internal/biz/foo.go b/internal/biz/foo.go\n" +
				"index 111..222 100644\n" +
				"--- a/internal/biz/foo.go\n" +
				"+++ b/internal/biz/foo.go\n" +
				"@@ -1,1 +1,1 @@\n-old\n+new\n",
			want: []PatchFileChange{{Path: "internal/biz/foo.go", Kind: PatchChangeModified}},
		},
		{
			name: "added file",
			diff: "diff --git a/internal/biz/new.go b/internal/biz/new.go\n" +
				"new file mode 100644\n" +
				"--- /dev/null\n" +
				"+++ b/internal/biz/new.go\n" +
				"@@ -0,0 +1,1 @@\n+package biz\n",
			want: []PatchFileChange{{Path: "internal/biz/new.go", Kind: PatchChangeAdded}},
		},
		{
			name: "deleted file",
			diff: "diff --git a/internal/biz/old.go b/internal/biz/old.go\n" +
				"deleted file mode 100644\n" +
				"--- a/internal/biz/old.go\n" +
				"+++ /dev/null\n" +
				"@@ -1,1 +0,0 @@\n-package biz\n",
			want: []PatchFileChange{{Path: "internal/biz/old.go", Kind: PatchChangeDeleted}},
		},
		{
			name: "renamed file",
			diff: "diff --git a/internal/biz/a.go b/internal/biz/b.go\n" +
				"similarity index 90%\n" +
				"rename from internal/biz/a.go\n" +
				"rename to internal/biz/b.go\n" +
				"--- a/internal/biz/a.go\n" +
				"+++ b/internal/biz/b.go\n",
			want: []PatchFileChange{{Path: "internal/biz/b.go", OldPath: "internal/biz/a.go", Kind: PatchChangeRenamed}},
		},
		{
			name: "multiple files",
			diff: "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n" +
				"diff --git a/y.ts b/y.ts\n--- a/y.ts\n+++ b/y.ts\n@@ -1 +1 @@\n-c\n+d\n",
			want: []PatchFileChange{
				{Path: "x.go", Kind: PatchChangeModified},
				{Path: "y.ts", Kind: PatchChangeModified},
			},
		},
		{name: "empty diff", diff: "", want: nil},
		{name: "garbage without headers", diff: "hello\nworld\n", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseUnifiedDiffFiles(tc.diff)
			if len(got) != len(tc.want) {
				t.Fatalf("ParseUnifiedDiffFiles() = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("change[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ── T2.4: protected file list (design D9) ────────────────────────────────────

func TestCheckProtectedFiles(t *testing.T) {
	cases := []struct {
		name    string
		changes []PatchFileChange
		wantHit []string // paths expected to be rejected
	}{
		{
			name:    "github workflow blocked",
			changes: []PatchFileChange{{Path: ".github/workflows/ci.yml", Kind: PatchChangeModified}},
			wantHit: []string{".github/workflows/ci.yml"},
		},
		{
			name:    "Makefile blocked",
			changes: []PatchFileChange{{Path: "Makefile", Kind: PatchChangeModified}},
			wantHit: []string{"Makefile"},
		},
		{
			name:    "go.mod and go.sum blocked",
			changes: []PatchFileChange{{Path: "go.mod", Kind: PatchChangeModified}, {Path: "go.sum", Kind: PatchChangeModified}},
			wantHit: []string{"go.mod", "go.sum"},
		},
		{
			name:    "existing proto modify blocked",
			changes: []PatchFileChange{{Path: "api/kratos/chat/v1/chat.proto", Kind: PatchChangeModified}},
			wantHit: []string{"api/kratos/chat/v1/chat.proto"},
		},
		{
			name:    "new proto file allowed",
			changes: []PatchFileChange{{Path: "api/kratos/self_improvement/v1/si.proto", Kind: PatchChangeAdded}},
			wantHit: nil,
		},
		{
			name:    "historical migration modify blocked",
			changes: []PatchFileChange{{Path: "internal/data/sql/migrations/20260101_init.sql", Kind: PatchChangeModified}},
			wantHit: []string{"internal/data/sql/migrations/20260101_init.sql"},
		},
		{
			name:    "new migration file allowed",
			changes: []PatchFileChange{{Path: "internal/data/sql/migrations/20261120_new.sql", Kind: PatchChangeAdded}},
			wantHit: nil,
		},
		{
			name:    "migration delete blocked",
			changes: []PatchFileChange{{Path: "internal/data/sql/migrations/20260101_init.sql", Kind: PatchChangeDeleted}},
			wantHit: []string{"internal/data/sql/migrations/20260101_init.sql"},
		},
		{
			name:    "wire_gen blocked",
			changes: []PatchFileChange{{Path: "cmd/admin/wire_gen.go", Kind: PatchChangeModified}},
			wantHit: []string{"cmd/admin/wire_gen.go"},
		},
		{
			name:    "ent generated code blocked",
			changes: []PatchFileChange{{Path: "internal/data/ent/client.go", Kind: PatchChangeModified}},
			wantHit: []string{"internal/data/ent/client.go"},
		},
		{
			name:    "ent schema handwritten allowed",
			changes: []PatchFileChange{{Path: "internal/data/ent/schema/self_improvement_run.go", Kind: PatchChangeModified}},
			wantHit: nil,
		},
		{
			name:    "renamed into protected path blocked",
			changes: []PatchFileChange{{Path: "Makefile", OldPath: "Makefile.bak", Kind: PatchChangeRenamed}},
			wantHit: []string{"Makefile"},
		},
		{
			name:    "renamed out of protected path blocked via old path",
			changes: []PatchFileChange{{Path: "ci.yml.disabled", OldPath: ".github/workflows/ci.yml", Kind: PatchChangeRenamed}},
			wantHit: []string{".github/workflows/ci.yml"},
		},
		{
			name:    "normal business file allowed",
			changes: []PatchFileChange{{Path: "internal/biz/chat_usecase.go", Kind: PatchChangeModified}},
			wantHit: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := CheckProtectedFiles(tc.changes, DefaultProtectedFileRules())
			if len(hits) != len(tc.wantHit) {
				t.Fatalf("CheckProtectedFiles() hits = %+v, want paths %v", hits, tc.wantHit)
			}
			for i, h := range hits {
				if h.Path != tc.wantHit[i] {
					t.Errorf("hit[%d].Path = %q, want %q", i, h.Path, tc.wantHit[i])
				}
				if h.Reason == "" {
					t.Errorf("hit[%d] missing reason", i)
				}
			}
		})
	}
}

// ── T2.6: diff size cap + sensitive content scan (SEL-08) ────────────────────

func TestComputeDiffStats(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n" +
		"--- a/x.go\n" +
		"+++ b/x.go\n" +
		"@@ -1,2 +1,3 @@\n" +
		" ctx := context.Background()\n" +
		"-oldCall(ctx)\n" +
		"+newCall(ctx)\n" +
		"+defer cleanup()\n" +
		"diff --git a/y.go b/y.go\n" +
		"new file mode 100644\n" +
		"--- /dev/null\n" +
		"+++ b/y.go\n" +
		"@@ -0,0 +1,1 @@\n" +
		"+package y\n"
	stats := ComputeDiffStats(diff)
	if stats.Files != 2 {
		t.Errorf("Files = %d, want 2", stats.Files)
	}
	if stats.Additions != 3 {
		t.Errorf("Additions = %d, want 3", stats.Additions)
	}
	if stats.Deletions != 1 {
		t.Errorf("Deletions = %d, want 1", stats.Deletions)
	}
}

func TestValidateDiffSize(t *testing.T) {
	mk := func(adds int) string {
		body := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -0,0 +1,N @@\n"
		for i := 0; i < adds; i++ {
			body += "+line\n"
		}
		return body
	}
	if err := ValidateDiffSize(mk(500), 500); err != nil {
		t.Errorf("500 additions within cap, got err %v", err)
	}
	if err := ValidateDiffSize(mk(501), 500); err == nil {
		t.Error("501 additions over cap, want error")
	}
	if err := ValidateDiffSize(mk(600), 0); err == nil {
		t.Error("maxLines=0 should use default cap and reject 600")
	}
	if err := ValidateDiffSize("", 500); err != nil {
		t.Errorf("empty diff should pass, got %v", err)
	}
}

func TestDetectSensitiveContent(t *testing.T) {
	cases := []struct {
		name string
		diff string
		want []string // expected hit kinds (order-insensitive)
	}{
		{
			name: "openai key",
			diff: "+const key = \"sk-abcdefghij1234567890\"\n",
			want: []string{"openai_key"},
		},
		{
			name: "aws akia",
			diff: "+aws := \"AKIAIOSFODNN7EXAMPLE\"\n",
			want: []string{"aws_access_key"},
		},
		{
			name: "private key block",
			diff: "+-----BEGIN RSA PRIVATE KEY-----\n+MIIEowIBAAKCAQEA\n+-----END RSA PRIVATE KEY-----\n",
			want: []string{"private_key"},
		},
		{
			name: "dsn with password",
			diff: "+dsn := \"postgres://admin:s3cret@db.internal:5432/app\"\n",
			want: []string{"dsn_password"},
		},
		{
			name: "github pat",
			diff: "+token := \"ghp_abcdefghijABCDEFGHIJ1234567890\"\n",
			want: []string{"github_pat"},
		},
		{
			name: "generic secret assignment",
			diff: "+password = \"sup3rS3cretP@ss\"\n",
			want: []string{"generic_secret"},
		},
		{
			name: "plain code no hit",
			diff: "+func handle(ctx context.Context) error {\n+\treturn nil\n+}\n",
			want: nil,
		},
		{
			name: "word-boundary: task-sk suffix not flagged",
			diff: "+kind := \"task-sk\"\n+mode := \"disk-usage\"\n",
			want: nil,
		},
		{
			name: "multiple kinds",
			diff: "+a := \"sk-abcdefghij1234567890\"\n+b := \"AKIAIOSFODNN7EXAMPLE\"\n",
			want: []string{"openai_key", "aws_access_key"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hits := DetectSensitiveContent(tc.diff)
			if len(hits) != len(tc.want) {
				t.Fatalf("DetectSensitiveContent() = %v, want kinds %v", hits, tc.want)
			}
			got := map[string]bool{}
			for _, h := range hits {
				got[h] = true
			}
			for _, w := range tc.want {
				if !got[w] {
					t.Errorf("missing kind %q in %v", w, hits)
				}
			}
		})
	}
}

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
