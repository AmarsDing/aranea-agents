package importer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func makeSkillMD(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\nSkill body content."
}

func makeSkillMDNoFrontmatter(name, desc string) string {
	return "# " + name + "\n\n" + desc + "\n\nSkill body content."
}

func TestValidateSkillPackage_ValidWithSKILLMD(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("My Skill", "A great skill")),
	}
	candidate, tags, _ := ValidateSkillPackage(files, "my-skill", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass, got %q", candidate.ValidationStatus)
	}
	if candidate.StatusIcon != "check" {
		t.Errorf("expected check, got %q", candidate.StatusIcon)
	}
	if candidate.Name != "My Skill" {
		t.Errorf("expected name 'My Skill', got %q", candidate.Name)
	}
	if candidate.Slug != "my-skill" {
		t.Errorf("expected slug 'my-skill', got %q", candidate.Slug)
	}
	if candidate.Description != "A great skill" {
		t.Errorf("expected description 'A great skill', got %q", candidate.Description)
	}
	if len(candidate.Blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(candidate.Blocks))
	}
	if len(candidate.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(candidate.Warnings))
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestValidateSkillPackage_BodyTooLongWarning(t *testing.T) {
	var b strings.Builder
	b.WriteString(makeSkillMD("Long Skill", "A distinct trigger-oriented description"))
	for i := 0; i < skillBodyWarnMaxLines; i++ {
		b.WriteString("extra line that does not change a decision\n")
	}
	files := map[string][]byte{"SKILL.md": []byte(b.String())}
	candidate, _, _ := ValidateSkillPackage(files, "long-skill", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Fatalf("oversized body must remain pass with warning, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, w := range candidate.Warnings {
		if w.Type == "body_too_long" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected body_too_long warning, got %+v", candidate.Warnings)
	}
}

func TestValidateSkillPackage_LowercaseSkillMD(t *testing.T) {
	files := map[string][]byte{
		"skill.md": []byte(makeSkillMD("Lower", "Lowercase filename")),
	}
	candidate, _, _ := ValidateSkillPackage(files, "lower", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass for lowercase skill.md, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_MissingSKILLMD(t *testing.T) {
	files := map[string][]byte{
		"README.md": []byte("no skill here"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "some-dir", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "missing_skill_md" {
			found = true
		}
	}
	if !found {
		t.Error("expected missing_skill_md block")
	}
}

func TestValidateSkillPackage_MissingName(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\ndescription: has desc but no name\n---\n\nBody."),
	}
	candidate, _, _ := ValidateSkillPackage(files, "dir-hint", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "invalid_format" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid_format block for missing name")
	}
}

func TestValidateSkillPackage_MissingDescription(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: HasName\n---\n\nBody."),
	}
	candidate, _, _ := ValidateSkillPackage(files, "dir-hint", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "invalid_format" {
			found = true
		}
	}
	if !found {
		t.Error("expected invalid_format block for missing description")
	}
}

func TestValidateSkillPackage_EmptyNameAndDesc(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: \ndescription: \n---\n\nBody."),
	}
	candidate, _, _ := ValidateSkillPackage(files, "fallback-dir", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	if candidate.Slug == "" {
		t.Error("expected non-empty slug from dirSlugHint fallback")
	}
}

func TestValidateSkillPackage_ShellScriptWarning(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Shell Skill", "Has shell script")),
		"setup.sh": []byte("#!/bin/bash\necho hi"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "shell-skill", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass (warning only), got %q", candidate.ValidationStatus)
	}
	found := false
	for _, w := range candidate.Warnings {
		if w.Type == "script_asset" {
			found = true
		}
	}
	if !found {
		t.Error("expected script_asset warning")
	}
}

func TestValidateSkillPackage_HighRiskFile(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":    []byte(makeSkillMD("Risky", "Has exe")),
		"payload.exe": []byte("binary"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "risky", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "high_risk_file" {
			found = true
		}
	}
	if !found {
		t.Error("expected high_risk_file block")
	}
}

func TestValidateSkillPackage_HighRiskFileBat(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Bat Skill", "Has bat file")),
		"run.bat":  []byte("@echo off"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "bat-skill", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "high_risk_file" {
			found = true
		}
	}
	if !found {
		t.Error("expected high_risk_file block for .bat")
	}
}

func TestValidateSkillPackage_HighRiskFileCmd(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":   []byte(makeSkillMD("Cmd Skill", "Has cmd file")),
		"script.cmd": []byte("echo bad"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "cmd-skill", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_HighRiskFileDll(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Dll Skill", "Has dll")),
		"lib.dll":  []byte("binary"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "dll-skill", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_HighRiskFilePs1(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":   []byte(makeSkillMD("Ps1 Skill", "Has ps1")),
		"script.ps1": []byte("Write-Host hi"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "ps1-skill", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_HighRiskFileSo(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("So Skill", "Has so")),
		"lib.so":   []byte("binary"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "so-skill", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_HighRiskFileDylib(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":  []byte(makeSkillMD("Dylib Skill", "Has dylib")),
		"lib.dylib": []byte("binary"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "dylib-skill", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_DuplicateName(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Existing Skill", "A skill")),
	}
	existing := []biz.SkillSimilaritySource{
		{Name: "Existing Skill", Slug: "existing-skill"},
	}
	candidate, _, _ := ValidateSkillPackage(files, "new-dir", existing, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block for duplicate name, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "duplicate_name" {
			found = true
		}
	}
	if !found {
		t.Error("expected duplicate_name block")
	}
}

func TestValidateSkillPackage_DuplicateSlug(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Different Name", "A skill")),
	}
	existing := []biz.SkillSimilaritySource{
		{Name: "Other Name", Slug: "different-name"},
	}
	candidate, _, _ := ValidateSkillPackage(files, "new-dir", existing, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block for duplicate slug, got %q", candidate.ValidationStatus)
	}
	found := false
	for _, b := range candidate.Blocks {
		if b.Type == "duplicate_name" {
			found = true
		}
	}
	if !found {
		t.Error("expected duplicate_name block for slug match")
	}
}

func TestValidateSkillPackage_SkipDuplicateCheck(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Existing Skill", "A skill")),
	}
	existing := []biz.SkillSimilaritySource{
		{Name: "Existing Skill", Slug: "existing-skill"},
	}
	candidate, _, _ := ValidateSkillPackage(files, "new-dir", existing, true)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass with skipDuplicateCheck=true, got %q", candidate.ValidationStatus)
	}
	for _, b := range candidate.Blocks {
		if b.Type == "duplicate_name" {
			t.Error("should not have duplicate_name block when skipDuplicateCheck=true")
		}
	}
}

func TestValidateSkillPackage_MultipleWarnings(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":  []byte(makeSkillMD("Multi Warn", "Multiple warnings")),
		"deploy.sh": []byte("#!/bin/bash\necho deploy"),
		"setup.sh":  []byte("#!/bin/bash\necho setup"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "multi-warn", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass (warnings only), got %q", candidate.ValidationStatus)
	}
	scriptWarnings := 0
	for _, w := range candidate.Warnings {
		if w.Type == "script_asset" {
			scriptWarnings++
		}
	}
	if scriptWarnings != 1 {
		t.Errorf("expected exactly 1 script_asset warning (deduplicated by type), got %d", scriptWarnings)
	}
}

func TestValidateSkillPackage_ValidWithExtraFiles(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":     []byte(makeSkillMD("Extra Files", "Has extras")),
		"examples.txt": []byte("example content"),
		"data.json":    []byte(`{"key": "value"}`),
	}
	candidate, _, _ := ValidateSkillPackage(files, "extra-files", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass, got %q", candidate.ValidationStatus)
	}
	if len(candidate.Blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(candidate.Blocks))
	}
	if len(candidate.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(candidate.Warnings))
	}
}

func TestValidateSkillPackage_BodyPreviewTruncated(t *testing.T) {
	longBody := strings.Repeat("x", 500)
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Long Skill", "Long desc") + longBody),
	}
	candidate, _, _ := ValidateSkillPackage(files, "long-skill", nil, false)
	if len(candidate.BodyPreview) > 243 {
		t.Errorf("expected body preview <= 243 chars (240 + '...'), got %d", len(candidate.BodyPreview))
	}
	if !strings.HasSuffix(candidate.BodyPreview, "...") {
		t.Error("expected body preview to end with '...'")
	}
}

func TestValidateSkillPackage_SlugFromDirHint(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\n---\n\n# Implicit Name\nSome desc here."),
	}
	candidate, _, _ := ValidateSkillPackage(files, "my-dir-hint", nil, false)
	if candidate.Slug == "" {
		t.Error("expected non-empty slug derived from dir hint or heading")
	}
}

func TestValidateSkillPackage_CandidateIDGenerated(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("ID Test", "Test")),
	}
	candidate, _, _ := ValidateSkillPackage(files, "id-test", nil, false)
	if candidate.CandidateID == "" {
		t.Error("expected non-empty CandidateID")
	}
}

func TestValidateSkillPackage_TagsParsed(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte("---\nname: Tagged\ndescription: Tagged skill\ntags: [coding, review]\n---\n\nBody."),
	}
	candidate, tags, _ := ValidateSkillPackage(files, "tagged", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass, got %q", candidate.ValidationStatus)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
	if tags[0].Name != "coding" {
		t.Errorf("expected first tag 'coding', got %q", tags[0].Name)
	}
	if tags[1].Name != "review" {
		t.Errorf("expected second tag 'review', got %q", tags[1].Name)
	}
}

func TestValidateSkillPackage_BlockAndWarningTogether(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":  []byte("---\nname: \ndescription: has desc\n---\n\nBody."),
		"deploy.sh": []byte("#!/bin/bash"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "mixed", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	hasInvalidFormat := false
	hasScriptWarning := false
	for _, b := range candidate.Blocks {
		if b.Type == "invalid_format" {
			hasInvalidFormat = true
		}
	}
	for _, w := range candidate.Warnings {
		if w.Type == "script_asset" {
			hasScriptWarning = true
		}
	}
	if !hasInvalidFormat {
		t.Error("expected invalid_format block")
	}
	if !hasScriptWarning {
		t.Error("expected script_asset warning alongside block")
	}
}

func TestValidateSkillPackage_HighRiskAndShellScript(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":    []byte(makeSkillMD("Risky Shell", "Both risky and shell")),
		"malware.exe": []byte("binary"),
		"deploy.sh":   []byte("#!/bin/bash"),
	}
	candidate, _, _ := ValidateSkillPackage(files, "risky-shell", nil, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block, got %q", candidate.ValidationStatus)
	}
	hasHighRisk := false
	hasScriptWarning := false
	for _, b := range candidate.Blocks {
		if b.Type == "high_risk_file" {
			hasHighRisk = true
		}
	}
	for _, w := range candidate.Warnings {
		if w.Type == "script_asset" {
			hasScriptWarning = true
		}
	}
	if !hasHighRisk {
		t.Error("expected high_risk_file block")
	}
	if !hasScriptWarning {
		t.Error("expected script_asset warning alongside high_risk_file block")
	}
}

func TestValidateSkillPackage_DuplicateNameCaseInsensitive(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("existing skill", "lowercase name")),
	}
	existing := []biz.SkillSimilaritySource{
		{Name: "Existing Skill", Slug: "other-slug"},
	}
	candidate, _, _ := ValidateSkillPackage(files, "new-dir", existing, false)
	if candidate.ValidationStatus != "block" {
		t.Errorf("expected block for case-insensitive duplicate name, got %q", candidate.ValidationStatus)
	}
}

func TestValidateSkillPackage_NoExistingSkills(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": []byte(makeSkillMD("Fresh Skill", "Brand new")),
	}
	candidate, _, _ := ValidateSkillPackage(files, "fresh", nil, false)
	if candidate.ValidationStatus != "pass" {
		t.Errorf("expected pass with no existing skills, got %q", candidate.ValidationStatus)
	}
}

func TestReadSkillDirFiles_ValidDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Test\nDesc"), 0o644)
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("hello"), 0o644)

	files, err := ReadSkillDirFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if _, ok := files["SKILL.md"]; !ok {
		t.Error("expected SKILL.md in result")
	}
	if _, ok := files["data.txt"]; !ok {
		t.Error("expected data.txt in result")
	}
}

func TestReadSkillDirFiles_Subdirectory(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0o755)
	os.WriteFile(filepath.Join(sub, "nested.md"), []byte("nested content"), 0o644)

	files, err := ReadSkillDirFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := files["sub/nested.md"]; !ok {
		t.Error("expected sub/nested.md in result with slash separator")
	}
}

func TestReadSkillDirFiles_NonExistentDirectory(t *testing.T) {
	_, err := ReadSkillDirFiles(filepath.Join(t.TempDir(), "nonexistent"))
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestReadSkillDirFiles_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	bigFile := filepath.Join(dir, "big.dat")
	bigData := make([]byte, 2*1024*1024+1)
	os.WriteFile(bigFile, bigData, 0o644)

	_, err := ReadSkillDirFiles(dir)
	if err == nil {
		t.Fatal("expected error for file too large")
	}
	if !errors.Is(err, ErrSkillFileTooLarge) {
		t.Errorf("expected ErrSkillFileTooLarge, got %v", err)
	}
}

func TestReadSkillDirFiles_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	files, err := ReadSkillDirFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files in empty dir, got %d", len(files))
	}
}

func TestReadSkillDirFiles_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# Test"), 0o644)

	files, err := ReadSkillDirFiles(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file (subdir should be skipped), got %d", len(files))
	}
}

func TestIsSafePath_NormalPaths(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"SKILL.md", true},
		{"sub/file.txt", true},
		{"a/b/c.md", true},
		{"readme", true},
	}
	for _, tc := range cases {
		got := isSafePath(tc.path)
		if got != tc.want {
			t.Errorf("isSafePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsSafePath_DotDot(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"../etc/passwd", false},
		{"a/../b", false},
		{"..", false},
	}
	for _, tc := range cases {
		got := isSafePath(tc.path)
		if got != tc.want {
			t.Errorf("isSafePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsSafePath_AbsolutePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/etc/passwd", false},
	}
	for _, tc := range cases {
		got := isSafePath(tc.path)
		if got != tc.want {
			t.Errorf("isSafePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestIsSafePath_BackslashPath(t *testing.T) {
	got := isSafePath("\\Windows\\System32")
	if got != false {
		t.Errorf("isSafePath(%q) = %v, want false", "\\Windows\\System32", got)
	}
}

func TestIsSafePath_EmptyString(t *testing.T) {
	got := isSafePath("")
	if got != false {
		t.Errorf("isSafePath('') = %v, want false", got)
	}
}

func TestIsSafePath_HiddenFile(t *testing.T) {
	got := isSafePath(".hidden")
	if got != true {
		t.Errorf("isSafePath(%q) = %v, want true (hidden files are safe)", ".hidden", got)
	}
}

func TestIsSafePath_DotInName(t *testing.T) {
	got := isSafePath("my.file.txt")
	if got != true {
		t.Errorf("isSafePath(%q) = %v, want true", "my.file.txt", got)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"My-Cool_Skill", "my-cool_skill"},
		{"UPPERCASE", "uppercase"},
		{"  spaces  ", "spaces"},
		{"special!@#chars", "special-chars"},
		{"a", "a"},
		{"123", "123"},
		{"Skill With   Multiple   Spaces", "skill-with-multiple-spaces"},
	}
	for _, tc := range cases {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestSlugify_EmptyString(t *testing.T) {
	got := slugify("")
	if got == "" {
		t.Error("slugify('') should return a fallback, got empty string")
	}
	if !strings.HasPrefix(got, "skill-") {
		t.Errorf("slugify('') should return 'skill-' prefix, got %q", got)
	}
}

func TestSlugify_OnlySpecialChars(t *testing.T) {
	got := slugify("!@#$%")
	if got == "" {
		t.Error("slugify('!@#$%') should return a fallback, got empty string")
	}
}

func TestSlugify_LeadingTrailingDashes(t *testing.T) {
	got := slugify("--hello--")
	if got != "hello" {
		t.Errorf("slugify('--hello--') = %q, want 'hello'", got)
	}
}

func TestHighRiskFileNames(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":  {},
		"app.exe":   {},
		"run.bat":   {},
		"script.sh": {},
		"data.json": {},
	}
	names := highRiskFileNames(files)
	if len(names) != 2 {
		t.Fatalf("expected 2 high-risk files, got %d: %v", len(names), names)
	}
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	if !seen["app.exe"] || !seen["run.bat"] {
		t.Errorf("expected app.exe and run.bat, got %v", names)
	}
}

func TestHighRiskFileNames_NoRiskyFiles(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md":   {},
		"readme.txt": {},
	}
	names := highRiskFileNames(files)
	if len(names) != 0 {
		t.Errorf("expected 0 high-risk files, got %d: %v", len(names), names)
	}
}

func TestHasShellScriptAsset(t *testing.T) {
	files := map[string][]byte{
		"deploy.sh": {},
	}
	if !hasShellScriptAsset(files) {
		t.Error("expected true for .sh file")
	}
}

func TestHasShellScriptAsset_NoShell(t *testing.T) {
	files := map[string][]byte{
		"SKILL.md": {},
	}
	if hasShellScriptAsset(files) {
		t.Error("expected false when no .sh files")
	}
}

func TestIsBlockedSkillFile(t *testing.T) {
	blocked := []string{"app.exe", "run.bat", "script.cmd", "task.ps1", "lib.dll", "lib.so", "lib.dylib"}
	for _, name := range blocked {
		if !isBlockedSkillFile(name) {
			t.Errorf("isBlockedSkillFile(%q) = false, want true", name)
		}
	}
}

func TestIsBlockedSkillFile_SafeFiles(t *testing.T) {
	safe := []string{"SKILL.md", "data.json", "script.sh", "image.png", "readme.txt"}
	for _, name := range safe {
		if isBlockedSkillFile(name) {
			t.Errorf("isBlockedSkillFile(%q) = true, want false", name)
		}
	}
}

func TestIsBlockedSkillFile_CaseInsensitive(t *testing.T) {
	if !isBlockedSkillFile("APP.EXE") {
		t.Error("isBlockedSkillFile should be case-insensitive")
	}
	if !isBlockedSkillFile("Run.Bat") {
		t.Error("isBlockedSkillFile should be case-insensitive")
	}
}
