package importer

import (
	"testing"
)

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
