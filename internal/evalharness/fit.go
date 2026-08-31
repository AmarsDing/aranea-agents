package evalharness

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// ArmVerdict is the eval-arm outcome. Inconclusive is a first-class result:
// a failed precondition must not be scored as pass (FIT-EVAL-1 / S10).
type ArmVerdict string

const (
	VerdictPass         ArmVerdict = "pass"
	VerdictFail         ArmVerdict = "fail"
	VerdictInconclusive ArmVerdict = "inconclusive"
)

// ToolfailArmVerdict is FIT-EVAL-1 for S10 toolfail: the fault must actually
// be injected before the "tool failed" branch is scored. Uninjected →
// inconclusive, never pass.
func ToolfailArmVerdict(injectionSucceeded bool, observedToolFailure bool) ArmVerdict {
	if !injectionSucceeded {
		return VerdictInconclusive
	}
	if observedToolFailure {
		return VerdictPass
	}
	return VerdictFail
}

// EmptyEvidenceFiles returns paths whose size is 0 (missing files should be
// recorded as size 0 by the caller). Any hit is an FIT-EVAL-1 failure.
func EmptyEvidenceFiles(sizeByPath map[string]int64) []string {
	var empty []string
	for path, size := range sizeByPath {
		if strings.TrimSpace(path) == "" {
			continue
		}
		if size <= 0 {
			empty = append(empty, path)
		}
	}
	return empty
}

// StatEvidenceDir builds sizeByPath for CheckEvidenceDir. Missing files
// are recorded as size 0 so they fail FIT-EVAL-1 the same as empty files.
func StatEvidenceDir(paths []string) map[string]int64 {
	out := make(map[string]int64, len(paths))
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil || fi.Size() <= 0 {
			out[p] = 0
			continue
		}
		out[p] = fi.Size()
	}
	return out
}

// CountSentences splits on CJK and Latin sentence terminators. Used by the
// S01 "压缩成三句话" instruction-following assertion.
func CountSentences(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	n := 0
	var buf strings.Builder
	flush := func() {
		s := strings.TrimSpace(buf.String())
		buf.Reset()
		if s == "" {
			return
		}
		// Drop leftover punctuation-only fragments.
		hasLetter := false
		for _, r := range s {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				hasLetter = true
				break
			}
		}
		if hasLetter {
			n++
		}
	}
	for _, r := range text {
		switch r {
		case '。', '！', '？', '.', '!', '?':
			flush()
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return n
}

// AssertSentenceCount reports FIT-EVAL-1 instruction-following failure.
func AssertSentenceCount(output string, want int) error {
	got := CountSentences(output)
	if got != want {
		return fmt.Errorf("FIT-EVAL-1: sentence count = %d, want %d", got, want)
	}
	return nil
}

// UnattendedEvalAutoApproveConfigured is the contract freeze for spawn HITL:
// unattended eval must use the existing env (ARANEA_TOOL_AUTO_APPROVE /
// KRATOS_TOOL_AUTO_APPROVE), not a second role whitelist.
func UnattendedEvalAutoApproveConfigured(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	return strings.TrimSpace(getenv("ARANEA_TOOL_AUTO_APPROVE")) == "1" ||
		strings.TrimSpace(getenv("KRATOS_TOOL_AUTO_APPROVE")) == "1"
}

// N5 fork remap (FIT-FORK-1): copy is strip inherited fk<8hex>- prefixes,
// then prepend exactly one new layer from the destination session id.
// Eval must not assert "renumber from the fork point" or stacked prefixes.
// Keep this aligned with internal/data/session_fork_repo.go; do not change
// remap in one place and the assertion in the other.
var evalForkPrefixRe = regexp.MustCompile(`^(?:fk[0-9a-fA-F]{8}-)+`)
var evalForkOneLayerRe = regexp.MustCompile(`^fk[0-9a-fA-F]{8}-`)

// StripForkIDPrefixes removes every leading fk<8hex>- generation prefix.
func StripForkIDPrefixes(id string) string {
	return evalForkPrefixRe.ReplaceAllString(id, "")
}

// ForkIDPrefixLayerCount counts leading fk<8hex>- layers (0 if none).
func ForkIDPrefixLayerCount(id string) int {
	n := 0
	for evalForkOneLayerRe.MatchString(id) {
		id = id[len("fk")+8+1:]
		n++
	}
	return n
}

// RemapForkedRecordID is the N5 id written into the forked session.
func RemapForkedRecordID(dstSessionID, sourceRecordID string) string {
	compact := strings.ReplaceAll(strings.TrimSpace(dstSessionID), "-", "")
	if len(compact) > 8 {
		compact = compact[:8]
	}
	return "fk" + compact + "-" + StripForkIDPrefixes(sourceRecordID)
}

// CheckForkedRecordIDContract is FIT-FORK-1: got must be single-layer
// remap, never a stacked prefix and never a sequential turn rewrite.
func CheckForkedRecordIDContract(dstSessionID, sourceRecordID, gotID string) error {
	want := RemapForkedRecordID(dstSessionID, sourceRecordID)
	if gotID != want {
		return fmt.Errorf("FIT-FORK-1: remapped id %q, want single-layer %q", gotID, want)
	}
	if ForkIDPrefixLayerCount(gotID) > 1 {
		return fmt.Errorf("FIT-FORK-1: stacked prefixes on %q", gotID)
	}
	return nil
}
