package biz

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ── Patch file parsing (73-self-iteration-v3, design D4/D9) ─────────────────
//
// Pure helpers that inspect a Patcher-produced unified diff before it enters
// the sandbox: which files it touches (D9 protection check), which go
// packages / frontend scope it affects (G2/G3 scoping), how large it is
// (D10 cap), and whether it carries sensitive material (SEL-08).

// PatchChangeKind classifies one file change inside a unified diff.
type PatchChangeKind string

const (
	PatchChangeAdded    PatchChangeKind = "added"
	PatchChangeModified PatchChangeKind = "modified"
	PatchChangeDeleted  PatchChangeKind = "deleted"
	PatchChangeRenamed  PatchChangeKind = "renamed"
)

// PatchFileChange is one file entry parsed from a unified diff.
type PatchFileChange struct {
	Path    string // repo-relative new path (for deleted: the removed path)
	OldPath string // repo-relative old path (renamed only)
	Kind    PatchChangeKind
}

// ParseUnifiedDiffFiles extracts the touched-file list from a unified diff by
// scanning the ---/+++ header pairs. Malformed or header-less input yields an
// empty slice (callers treat that as "no files", not an error).
func ParseUnifiedDiffFiles(diff string) []PatchFileChange {
	var changes []PatchFileChange
	var oldPath string
	var haveOld bool
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "--- "):
			oldPath = stripDiffPathPrefix(strings.TrimSpace(strings.TrimPrefix(line, "--- ")))
			haveOld = true
		case strings.HasPrefix(line, "+++ ") && haveOld:
			newPath := stripDiffPathPrefix(strings.TrimSpace(strings.TrimPrefix(line, "+++ ")))
			changes = append(changes, classifyDiffFile(oldPath, newPath))
			haveOld = false
		}
	}
	return changes
}

// stripDiffPathPrefix removes the a/ or b/ prefix git adds to diff paths and
// drops surrounding quotes (git quotes paths with spaces/non-ASCII).
func stripDiffPathPrefix(p string) string {
	p = strings.Trim(p, `"`)
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

func classifyDiffFile(oldPath, newPath string) PatchFileChange {
	const devNull = "/dev/null"
	switch {
	case oldPath == devNull:
		return PatchFileChange{Path: newPath, Kind: PatchChangeAdded}
	case newPath == devNull:
		return PatchFileChange{Path: oldPath, Kind: PatchChangeDeleted}
	case oldPath != newPath:
		return PatchFileChange{Path: newPath, OldPath: oldPath, Kind: PatchChangeRenamed}
	default:
		return PatchFileChange{Path: newPath, Kind: PatchChangeModified}
	}
}

// ── Affected scope derivation (design D4: G2/G3 scoping) ────────────────────

// DeriveAffectedScopes maps touched files to the go package patterns that
// must be built/tested and whether the web frontend is affected.
//
// Rules: `<dir>/foo.go` → `./<dir>/...`; root-level go files → `./`;
// anything under web/ flips the web flag. Patterns are deduped and sorted.
func DeriveAffectedScopes(changes []PatchFileChange) (goPkgs []string, webScope bool) {
	seen := map[string]bool{}
	for _, c := range changes {
		p := c.Path
		if p == "" {
			p = c.OldPath
		}
		if strings.HasPrefix(p, "web/") {
			webScope = true
			continue
		}
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		dir := path.Dir(p)
		pkg := "./"
		if dir != "." {
			pkg = "./" + dir + "/..."
		}
		if !seen[pkg] {
			seen[pkg] = true
			goPkgs = append(goPkgs, pkg)
		}
	}
	sort.Strings(goPkgs)
	return goPkgs, webScope
}

// ── Protected file list (design D9) ─────────────────────────────────────────

// ProtectedFileRule blocks patch changes against a doublestar glob. Added
// files may be exempted (AllowAdded) for append-only areas such as DDL
// migrations and new proto files.
type ProtectedFileRule struct {
	Glob       string
	AllowAdded bool
	Reason     string
}

// ProtectedFileHit records one protected-file violation.
type ProtectedFileHit struct {
	Path   string
	Rule   string
	Reason string
}

// DefaultProtectedFileRules returns the D9 protection list.
func DefaultProtectedFileRules() []ProtectedFileRule {
	return []ProtectedFileRule{
		{Glob: ".github/workflows/**", Reason: "CI 工作流禁止自动修改"},
		{Glob: "Makefile", Reason: "构建入口禁止自动修改"},
		{Glob: "go.mod", Reason: "依赖清单禁止自动修改"},
		{Glob: "go.sum", Reason: "依赖锁定禁止自动修改"},
		{Glob: "api/kratos/**/*.proto", AllowAdded: true, Reason: "已有 proto 禁止自动修改（允许新增 message/service 文件）"},
		{Glob: "internal/data/sql/migrations/**", AllowAdded: true, Reason: "历史迁移文件禁止修改/删除（允许新增迁移）"},
		{Glob: "cmd/admin/wire_gen.go", Reason: "wire 生成物禁止手改（须走 wire regenerate）"},
		{Glob: "internal/data/ent/*.go", Reason: "Ent 生成物禁止手改（须走 go generate）"},
	}
}

// CheckProtectedFiles returns every change that violates the protection
// rules. An empty result means the patch may proceed to verification.
// Renames are checked on both endpoints: renaming into or out of a protected
// path is equally forbidden.
func CheckProtectedFiles(changes []PatchFileChange, rules []ProtectedFileRule) []ProtectedFileHit {
	var hits []ProtectedFileHit
	for _, c := range changes {
		for _, r := range rules {
			if c.Kind == PatchChangeAdded && r.AllowAdded {
				continue
			}
			if hit, ok := matchProtected(c.Path, r); ok {
				hits = append(hits, hit)
				continue
			}
			if c.Kind == PatchChangeRenamed {
				if hit, ok := matchProtected(c.OldPath, r); ok {
					hits = append(hits, hit)
				}
			}
		}
	}
	return hits
}

func matchProtected(p string, r ProtectedFileRule) (ProtectedFileHit, bool) {
	if p == "" {
		return ProtectedFileHit{}, false
	}
	ok, err := doublestar.Match(r.Glob, p)
	if err != nil || !ok {
		return ProtectedFileHit{}, false
	}
	return ProtectedFileHit{Path: p, Rule: r.Glob, Reason: r.Reason}, true
}

// ── Diff size cap (design D10: patch.max_diff_lines) ────────────────────────

// DefaultMaxDiffLines is the governance cap on patch size (design D10).
const DefaultMaxDiffLines = 500

// ComputeDiffStats counts files / added / removed lines in a unified diff.
// The +++/--- file headers are not counted as content lines.
func ComputeDiffStats(diff string) DiffStats {
	var stats DiffStats
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			stats.Files++
		case strings.HasPrefix(line, "+++"):
			// file header, not content
		case strings.HasPrefix(line, "---"):
			// file header, not content
		case strings.HasPrefix(line, "+"):
			stats.Additions++
		case strings.HasPrefix(line, "-"):
			stats.Deletions++
		}
	}
	return stats
}

// ValidateDiffSize rejects patches whose changed-line count (additions +
// deletions) exceeds maxLines. maxLines <= 0 falls back to DefaultMaxDiffLines.
func ValidateDiffSize(diff string, maxLines int) error {
	if maxLines <= 0 {
		maxLines = DefaultMaxDiffLines
	}
	stats := ComputeDiffStats(diff)
	if total := stats.Additions + stats.Deletions; total > maxLines {
		return fmt.Errorf("diff 规模超限：变更 %d 行（+%d/-%d），上限 %d 行",
			total, stats.Additions, stats.Deletions, maxLines)
	}
	return nil
}

// ── Sensitive content scan (SEL-08, reused from V2 preview patterns) ────────
//
// The patterns mirror internal/tools/preview's redaction set. They are
// duplicated here deliberately: biz must not depend on the tools layer, and
// the scan returns hit *kinds* (never the matched secret material) so reports
// stay leak-free.

var sensitivePatterns = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"openai_key", regexp.MustCompile(`\bsk-[a-zA-Z0-9]{10,}\b`)},
	{"anthropic_key", regexp.MustCompile(`\bsk-ant-[a-zA-Z0-9\-]{10,}\b`)},
	{"xai_key", regexp.MustCompile(`\bxai-[a-zA-Z0-9]{8,}\b`)},
	{"aws_access_key", regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`)},
	{"github_pat", regexp.MustCompile(`\bghp_[a-zA-Z0-9]{20,}\b`)},
	{"google_api_key", regexp.MustCompile(`\bAIza[0-9A-Za-z\-_]{30,}\b`)},
	{"slack_token", regexp.MustCompile(`\bxox[bpar]-[a-zA-Z0-9\-]{10,}\b`)},
	{"stripe_key", regexp.MustCompile(`\b[sr]k_(live|test)_[a-zA-Z0-9]{10,}\b`)},
	{"jwt", regexp.MustCompile(`\beyJ[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+\.[a-zA-Z0-9\-_]+\b`)},
	{"private_key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"dsn_password", regexp.MustCompile(`(?i)(postgres|mysql|redis|mongodb|amqp|nats)://[^:]+:[^@]+@`)},
	{"generic_secret", regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password|authorization|bearer)\s*[:=]\s*\S{8,}`)},
}

// DetectSensitiveContent scans patch content for credential-like material and
// returns the deduplicated list of hit kinds (e.g. "openai_key"). Any non-empty
// result must block the patch (SEL-08).
func DetectSensitiveContent(diff string) []string {
	var kinds []string
	seen := map[string]bool{}
	for _, p := range sensitivePatterns {
		if seen[p.kind] {
			continue
		}
		if p.re.MatchString(diff) {
			seen[p.kind] = true
			kinds = append(kinds, p.kind)
		}
	}
	return kinds
}
