package pkginstall

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstallSkillFromURLSubpathZipsTempPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "skill", "SKILL.md"), []byte("# Test Skill\n"))
	git(t, repo, "init")
	git(t, repo, "add", ".")
	git(t, repo, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")

	var sawFile bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/skills/import" {
			http.NotFound(w, r)
			return
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader() error = %v", err)
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			if part.FormName() == "file" && part.FileName() != "" {
				sawFile = true
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ins := &Installer{APIURL: srv.URL, Quiet: true}
	result, err := ins.Install("", &Manifest{
		Version:  1,
		Metadata: ManifestMetadata{Name: "pkg"},
		Spec: ManifestSpec{Skills: []SkillSpec{{
			URL:     repo,
			Subpath: "skill",
		}}},
	})
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("Install() errors = %v", result.Errors)
	}
	if !sawFile {
		t.Fatal("skills/import did not receive a file part")
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
