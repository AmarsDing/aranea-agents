package importer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/pkg/strutil"
)

// DirectorySlugMismatch reports when a folder name does not match parsed slug.
func DirectorySlugMismatch(dirSlug, candidateSlug string) *biz.SkillImportIssue {
	if SlugifyOrRandom(dirSlug) != SlugifyOrRandom(candidateSlug) {
		return &biz.SkillImportIssue{
			Type:    "directory_slug_mismatch",
			Message: fmt.Sprintf("directory name %q must match skill slug %q", dirSlug, candidateSlug),
		}
	}
	return nil
}

func ValidateSkillPackage(files map[string][]byte, dirSlugHint string, existing []biz.SkillSimilaritySource, skipDuplicateCheck bool) (biz.SkillImportCandidate, []biz.SkillTag, []string) {
	bodyBytes, ok := files["SKILL.md"]
	if !ok {
		bodyBytes, ok = files["skill.md"]
	}
	if !ok {
		return biz.SkillImportCandidate{
			CandidateID:      newID(),
			ValidationStatus: "block",
			StatusIcon:       "block",
			Blocks:           []biz.SkillImportIssue{{Type: "missing_skill_md", Message: "SKILL.md is required"}},
		}, nil, nil
	}
	body := string(bodyBytes)
	parsed := manifest.Parse(body)
	name := parsed.Name
	desc := parsed.Description
	tags := parsed.Tags
	triggers := parsed.Triggers
	slug := slugify(strutil.FirstNonEmpty(name, pathBaseSkill(dirSlugHint)))
	if slug == "" {
		slug = slugify(pathBaseSkill(dirSlugHint))
	}
	candidate := biz.SkillImportCandidate{
		CandidateID:      newID(),
		Name:             name,
		Slug:             slug,
		Description:      desc,
		BodyPreview:      truncateRunes(body, 240),
		ValidationStatus: "pass",
		StatusIcon:       "check",
		Warnings:         []biz.SkillImportIssue{},
		Blocks:           []biz.SkillImportIssue{},
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(desc) == "" {
		candidate.ValidationStatus = "block"
		candidate.StatusIcon = "block"
		candidate.Blocks = append(candidate.Blocks, biz.SkillImportIssue{Type: "invalid_format", Message: "SKILL.md must include a title/name and description"})
	}
	if hasShellScriptAsset(files) {
		candidate.Warnings = append(candidate.Warnings, biz.SkillImportIssue{Type: "script_asset", Message: "包含 .sh 脚本文件，请确认该脚本在 Skill 运行时是否必要"})
	}
	if highRiskFiles := highRiskFileNames(files); len(highRiskFiles) > 0 {
		candidate.ValidationStatus = "block"
		candidate.StatusIcon = "security"
		candidate.Blocks = append(candidate.Blocks, biz.SkillImportIssue{
			Type:    "high_risk_file",
			Message: "包含高风险文件，请确认是否允许导入该 Skill: " + strings.Join(highRiskFiles, ", "),
		})
	}
	if !skipDuplicateCheck {
		for _, item := range existing {
			if strings.EqualFold(item.Name, name) || strings.EqualFold(item.Slug, slug) {
				candidate.ValidationStatus = "block"
				candidate.StatusIcon = "block"
				candidate.Blocks = append(candidate.Blocks, biz.SkillImportIssue{Type: "duplicate_name", Message: "Skill name or slug already exists"})
				break
			}
		}
	}
	appendSkillAuthoringWarnings(&candidate, body)
	return candidate, tags, triggers
}

const (
	// skillBodyWarnMaxBytes is the soft SKILL.md size after which import
	// warns. Progressive disclosure wants a short decision-only body;
	// references/ and scripts/ carry the rest.
	skillBodyWarnMaxBytes = 12 * 1024
	// skillBodyWarnMaxLines is the soft line budget for SKILL.md.
	skillBodyWarnMaxLines = 500
)

func appendSkillAuthoringWarnings(candidate *biz.SkillImportCandidate, body string) {
	if candidate == nil {
		return
	}
	lines := strings.Count(body, "\n") + 1
	if len(body) > skillBodyWarnMaxBytes || lines > skillBodyWarnMaxLines {
		candidate.Warnings = append(candidate.Warnings, biz.SkillImportIssue{
			Type:    "body_too_long",
			Message: fmt.Sprintf("SKILL.md is %d bytes / %d lines; keep the body under %d bytes or %d lines and move details to references/ or scripts/ (only write what changes a decision)", len(body), lines, skillBodyWarnMaxBytes, skillBodyWarnMaxLines),
		})
	}
	desc := strings.TrimSpace(candidate.Description)
	name := strings.TrimSpace(candidate.Name)
	if desc != "" && name != "" && strings.EqualFold(desc, name) {
		candidate.Warnings = append(candidate.Warnings, biz.SkillImportIssue{
			Type:    "description_not_distinct",
			Message: "description matches name; write a trigger-oriented description that distinguishes this skill from others",
		})
	}
}

// ReadSkillDirFiles reads regular files under dir into a slash-separated relative map.
func ReadSkillDirFiles(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isSafePath(rel) {
			return unsafePathError(ErrUnsafeRelPath, rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) > 2*1024*1024 {
			return detailErr(ErrSkillFileTooLarge, "skill file too large: "+rel)
		}
		out[rel] = data
		return nil
	})
	return out, err
}

func isSafePath(rel string) bool {
	if rel == "" {
		return false
	}
	cleaned := filepath.ToSlash(filepath.Clean(rel))
	if cleaned != rel {
		return false
	}
	if strings.Contains(rel, "..") {
		return false
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "\\") {
		return false
	}
	return true
}
