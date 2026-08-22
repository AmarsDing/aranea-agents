package biz

import (
	"strings"
	"testing"
)

// ── Prompt contracts (design D5) ────────────────────────────────────────────

func TestSIMetaAgentPrompts_ContainOutputContract(t *testing.T) {
	cases := []struct {
		name       string
		prompt     string
		mustSubstr []string
	}{
		{
			name:   "analyst",
			prompt: SIAnalystSystemPrompt,
			mustSubstr: []string{
				"root_cause", "affected_files", "impact_scope", "fix_strategy", "confidence",
				"local", "module", "global",
			},
		},
		{
			name:   "patcher",
			prompt: SIPatcherSystemPrompt,
			mustSubstr: []string{
				"diff", "files", "additions", "deletions", "kind",
				"code", "config", "prompt", "docs", "test",
				"unified diff", "patcher_fs_read", "patcher_git_diff",
			},
		},
		{
			name:       "verifier",
			prompt:     SIVerifierSystemPrompt,
			mustSubstr: []string{"gates", "gate", "passed", "output"},
		},
		{
			name:       "critic",
			prompt:     SICriticSystemPrompt,
			mustSubstr: []string{"is_safe", "risk_level", "concerns", "suggestion", "low", "medium", "high"},
		},
	}
	for _, c := range cases {
		if strings.TrimSpace(c.prompt) == "" {
			t.Errorf("%s: prompt is empty", c.name)
			continue
		}
		for _, sub := range c.mustSubstr {
			if !strings.Contains(c.prompt, sub) {
				t.Errorf("%s: prompt missing contract token %q", c.name, sub)
			}
		}
	}
}

func TestSIPrompts_ClaimWiredTools(t *testing.T) {
	if !strings.Contains(SIPatcherSystemPrompt, "patcher_fs_write") ||
		!strings.Contains(SIPatcherSystemPrompt, "patcher_git_diff") {
		t.Error("Patcher prompt must name the wired worktree tools")
	}
	if !strings.Contains(SIAnalystSystemPrompt, "patcher_fs_read") ||
		strings.Contains(SIAnalystSystemPrompt, "patcher_fs_write") {
		t.Error("Analyst prompt must offer read-only tools only")
	}
}

func TestSIMetaAgentIDs_Unique(t *testing.T) {
	ids := []string{
		SIAgentObserver, SIAgentAnalyst, SIAgentPatcher,
		SIAgentVerifier, SIAgentCritic, SIAgentGovernor, SIAgentApplier,
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" {
			t.Fatal("agent id must not be empty")
		}
		if seen[id] {
			t.Fatalf("duplicate agent id %q", id)
		}
		seen[id] = true
	}
}

// ── Diagnosis parsing ────────────────────────────────────────────────────────

func TestParseDiagnosisJSON(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string // substring; empty = expect success
		check   func(t *testing.T, d *Diagnosis)
	}{
		{
			name: "valid full",
			raw:  `{"root_cause":"nil deref in x","affected_files":["internal/biz/x.go"],"impact_scope":"local","fix_strategy":"guard nil","confidence":0.8}`,
			check: func(t *testing.T, d *Diagnosis) {
				if d.RootCause != "nil deref in x" || d.Confidence != 0.8 || d.ImpactScope != "local" {
					t.Errorf("unexpected diagnosis: %+v", d)
				}
			},
		},
		{
			name: "fenced json tolerated",
			raw:  "```json\n{\"root_cause\":\"r\",\"affected_files\":[],\"impact_scope\":\"module\",\"fix_strategy\":\"f\",\"confidence\":0.5}\n```",
			check: func(t *testing.T, d *Diagnosis) {
				if d.ImpactScope != "module" {
					t.Errorf("ImpactScope = %q", d.ImpactScope)
				}
			},
		},
		{name: "empty", raw: ``, wantErr: "empty"},
		{name: "invalid json", raw: `{not json`, wantErr: "invalid"},
		{name: "missing root_cause", raw: `{"impact_scope":"local","fix_strategy":"f","confidence":0.9}`, wantErr: "root_cause"},
		{name: "missing fix_strategy", raw: `{"root_cause":"r","impact_scope":"local","confidence":0.9}`, wantErr: "fix_strategy"},
		{name: "bad impact_scope", raw: `{"root_cause":"r","impact_scope":"cosmic","fix_strategy":"f","confidence":0.9}`, wantErr: "impact_scope"},
		{name: "confidence above 1", raw: `{"root_cause":"r","impact_scope":"local","fix_strategy":"f","confidence":1.2}`, wantErr: "confidence"},
		{name: "confidence negative", raw: `{"root_cause":"r","impact_scope":"local","fix_strategy":"f","confidence":-0.1}`, wantErr: "confidence"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := ParseDiagnosisJSON(c.raw)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil (d=%+v)", c.wantErr, d)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("error %q missing %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.check != nil {
				c.check(t, d)
			}
		})
	}
}

// ── PatcherOutput parsing ────────────────────────────────────────────────────

const siTestDiff = `diff --git a/internal/biz/x.go b/internal/biz/x.go
--- a/internal/biz/x.go
+++ b/internal/biz/x.go
@@ -1,2 +1,3 @@
 package biz
+// guard
 func x() {}
`

func TestParsePatcherOutputJSON(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
		check   func(t *testing.T, p *PatcherOutput)
	}{
		{
			name: "valid with declared stats normalized to computed",
			raw:  `{"diff":"` + strings.ReplaceAll(strings.ReplaceAll(siTestDiff, "\\", "\\\\"), "\n", "\\n") + `","files":99,"additions":99,"deletions":99,"kind":"code","summary":"add guard"}`,
			check: func(t *testing.T, p *PatcherOutput) {
				// Declared stats (99) must be normalized to the diff-derived truth.
				if p.Files != 1 || p.Additions != 1 || p.Deletions != 0 {
					t.Errorf("stats not normalized: %+v", p)
				}
				if p.Kind != PatchKindCode || p.Summary != "add guard" {
					t.Errorf("unexpected patcher output: %+v", p)
				}
			},
		},
		{name: "empty diff", raw: `{"diff":"","kind":"code"}`, wantErr: "diff"},
		{name: "bad kind", raw: `{"diff":"x","kind":"exploit"}`, wantErr: "kind"},
		{name: "invalid json", raw: `{`, wantErr: "invalid"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := ParsePatcherOutputJSON(c.raw)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("error %q missing %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.check != nil {
				c.check(t, p)
			}
		})
	}
}

// ── CriticReport parsing ─────────────────────────────────────────────────────

func TestParseCriticReportJSON(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
		check   func(t *testing.T, r *CriticReport)
	}{
		{
			name: "safe low",
			raw:  `{"is_safe":true,"risk_level":"low","concerns":[],"suggestion":"ok"}`,
			check: func(t *testing.T, r *CriticReport) {
				if !r.IsSafe || r.RiskLevel != "low" {
					t.Errorf("unexpected report: %+v", r)
				}
			},
		},
		{
			name: "unsafe with concerns",
			raw:  `{"is_safe":false,"risk_level":"high","concerns":["touches ent schema"],"suggestion":"manual review"}`,
			check: func(t *testing.T, r *CriticReport) {
				if r.IsSafe || len(r.Concerns) != 1 {
					t.Errorf("unexpected report: %+v", r)
				}
			},
		},
		{name: "bad risk level", raw: `{"is_safe":true,"risk_level":"fatal"}`, wantErr: "risk_level"},
		{name: "unsafe without concerns", raw: `{"is_safe":false,"risk_level":"high","concerns":[]}`, wantErr: "concerns"},
		{name: "empty", raw: ``, wantErr: "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := ParseCriticReportJSON(c.raw)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("error %q missing %q", err.Error(), c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.check != nil {
				c.check(t, r)
			}
		})
	}
}
