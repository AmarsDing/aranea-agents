package importer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
)

var fallbackIDCounter uint64

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&fallbackIDCounter, 1)
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405"))) + hex.EncodeToString([]byte{byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

func firstNonEmptyString(values ...string) string {
	return strutil.FirstNonEmpty(values...)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9\-_]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_")
	if value == "" {
		value = fmt.Sprintf("skill-%d", len(value))
	}
	return value
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

func parseSkillMarkdown(body string) (string, string, []biz.SkillTag) {
	name := ""
	description := ""
	tags := []biz.SkillTag{}
	lines := strings.Split(body, "\n")
	inFrontmatter := false
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFrontmatter = true
		for _, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" {
				break
			}
			if key, value, ok := splitMetaLine(trimmed); ok {
				switch strings.ToLower(key) {
				case "name", "title":
					name = strings.Trim(value, `"'`)
				case "description", "summary":
					description = strings.Trim(value, `"'`)
				case "tags":
					for _, tag := range strings.Split(strings.Trim(value, "[]"), ",") {
						tag = strings.Trim(strings.TrimSpace(tag), `"'`)
						if tag != "" {
							tags = append(tags, biz.SkillTag{Name: tag, Source: "user"})
						}
					}
				}
			}
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if name == "" && strings.HasPrefix(trimmed, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if description == "" && !inFrontmatter && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			description = trimmed
		}
	}
	return name, description, tags
}

func splitMetaLine(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
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
