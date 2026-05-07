package importer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
)

// ValidateSkillPackage runs structural checks shared by ZIP import and disk sync.
// When skipDuplicateCheck is false, existing DB names/slugs block import (ZIP flow).
// Disk sync sets skipDuplicateCheck true and relies on Upsert by slug.
// Returns parsed tags from SKILL.md frontmatter / headings.
func ValidateSkillPackage(files map[string][]byte, dirSlugHint string, existing []biz.SkillSimilaritySource, skipDuplicateCheck bool) (biz.SkillImportCandidate, []biz.SkillTag) {
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
		}, nil
	}
	body := string(bodyBytes)
	name, desc, tags := parseSkillMarkdown(body)
	slug := slugify(firstNonEmptyString(name, pathBaseSkill(dirSlugHint)))
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
		candidate.Warnings = append(candidate.Warnings, biz.SkillImportIssue{Type: "script_asset", Message: "?? .sh ????????? Skill ???????????"})
	}
	if highRiskFiles := highRiskFileNames(files); len(highRiskFiles) > 0 {
		candidate.ValidationStatus = "block"
		candidate.StatusIcon = "security"
		candidate.Blocks = append(candidate.Blocks, biz.SkillImportIssue{
			Type:    "high_risk_file",
			Message: "?????????????????????? Skill?" + strings.Join(highRiskFiles, ", "),
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
	return candidate, tags
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
		if rel == "" || strings.Contains(rel, "..") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(data) > 2*1024*1024 {
			return validationError("skill file too large: " + rel)
		}
		out[rel] = data
		return nil
	})
	return out, err
}
