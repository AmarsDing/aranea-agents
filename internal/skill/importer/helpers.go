package importer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
)

var fallbackIDCounter uint64

// slugNormalizeRe matches any character that is not a lowercase letter, digit,
// hyphen, or underscore. Used by slugify to produce URL-safe slugs.
var slugNormalizeRe = regexp.MustCompile(`[^a-z0-9\-_]+`)

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&fallbackIDCounter, 1)
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405"))) + hex.EncodeToString([]byte{byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugNormalizeRe.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if value == "" {
		value = "skill-" + newID()[:8]
	}
	return value
}

// SlugifyOrRandom normalizes a value to a slug, generating a random suffix
// if the result would be empty. Use this when creating new slugs (e.g., from
// directory names or parsed skill names). For matching existing slugs, use
// biz.NormalizeSkillSlug instead (returns empty for empty input).
func SlugifyOrRandom(value string) string {
	return slugify(value)
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func decodeModelJSON(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	return json.Unmarshal([]byte(raw), out)
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func validationError(msg string) error {
	return errors.New(msg)
}

// ensurePathWithin verifies that joining relName under absBase stays within absBase
// (defends against zipslip, including Windows drive prefixes, absolute paths, and
// path-traversal escapes after filepath.Clean). TPM-P1-07.
func ensurePathWithin(absBase, relName string) error {
	if filepath.IsAbs(relName) {
		return unsafePathError(ErrUnsafePathAbsolute, relName)
	}
	cleaned := filepath.Clean(relName)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || strings.Contains(cleaned, "..") {
		return unsafePathError(ErrUnsafePathTraversal, relName)
	}
	joined := filepath.Join(absBase, cleaned)
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return detailErr(ErrResolvePath, err.Error())
	}
	rel, err := filepath.Rel(absBase, absJoined)
	if err != nil {
		return detailErr(ErrRelPath, err.Error())
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return unsafePathError(ErrUnsafePathEscapes, relName)
	}
	return nil
}

func skillZipGroupPath(name string) (string, string) {
	name = filepath.ToSlash(name)
	name = strings.Trim(name, "/")
	if name == "" {
		return ".", ""
	}
	parts := strings.Split(name, "/")
	if len(parts) == 1 {
		return ".", parts[0]
	}
	return parts[0], strings.Join(parts[1:], "/")
}

func pathBaseSkill(name string) string {
	name = strings.Trim(filepath.ToSlash(name), "/")
	if name == "" || name == "." {
		return ""
	}
	parts := strings.Split(name, "/")
	return parts[len(parts)-1]
}

func isBlockedSkillFile(name string) bool {
	lower := strings.ToLower(name)
	blocked := []string{".exe", ".bat", ".cmd", ".ps1", ".dll", ".so", ".dylib"}
	for _, ext := range blocked {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func hasShellScriptAsset(files map[string][]byte) bool {
	for name := range files {
		if strings.HasSuffix(strings.ToLower(name), ".sh") {
			return true
		}
	}
	return false
}

func highRiskFileNames(files map[string][]byte) []string {
	names := []string{}
	for name := range files {
		if isBlockedSkillFile(name) {
			names = append(names, name)
		}
	}
	return names
}

func summarizeImportStatus(candidates []biz.SkillImportCandidate, groups []biz.SkillConflictGroup) string {
	for _, candidate := range candidates {
		if candidate.ValidationStatus == "block" {
			return "block"
		}
	}
	if len(groups) > 0 {
		return "warn"
	}
	return "pass"
}

func importBlockMessages(candidates []biz.SkillImportCandidate) []string {
	messages := []string{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		for _, block := range candidate.Blocks {
			message := strings.TrimSpace(block.Message)
			if message == "" || seen[message] {
				continue
			}
			seen[message] = true
			messages = append(messages, message)
		}
	}
	return messages
}

func candidateIDsForGroup(groups []biz.SkillConflictGroup, groupID string) []string {
	for _, group := range groups {
		if group.GroupID == groupID {
			return group.CandidateIDs
		}
	}
	return nil
}
