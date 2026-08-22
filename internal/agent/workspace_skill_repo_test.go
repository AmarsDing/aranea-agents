package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aranea-agents/internal/agent/reposkills"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

type stubSkillRepo struct {
	summaries []trpcskill.Summary
	skills    map[string]*trpcskill.Skill
}

func (s stubSkillRepo) Summaries() []trpcskill.Summary { return s.summaries }
func (s stubSkillRepo) Get(name string) (*trpcskill.Skill, error) {
	if sk, ok := s.skills[name]; ok {
		return sk, nil
	}
	return nil, fmt.Errorf("skill %q not found", name)
}
func (s stubSkillRepo) Path(name string) (string, error) {
	return "", fmt.Errorf("skill %q not found", name)
}

func TestWorkspaceSkillRepo_GetFallsBackToDisk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "xlsx-review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: XLSX Review\ndescription: Review spreadsheets.\n---\n\nDo not email raw xlsx.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := &workspaceSkillRepo{
		inner: stubSkillRepo{skills: map[string]*trpcskill.Skill{}},
		cwd:   root,
		roots: []string{root},
	}
	sk, err := repo.Get("xlsx-review")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if sk.Summary.Name != "xlsx-review" {
		t.Fatalf("name = %q", sk.Summary.Name)
	}
	if !strings.Contains(sk.Body, "Do not email") {
		t.Fatalf("body = %q", sk.Body)
	}
	p, err := repo.Path("xlsx-review")
	if err != nil || p != dir {
		t.Fatalf("Path = %q, %v", p, err)
	}
	found := false
	for _, s := range repo.Summaries() {
		if s.Name == "xlsx-review" && strings.Contains(s.Description, "workspace") {
			found = true
		}
	}
	if !found {
		t.Fatalf("summaries missing workspace skill: %+v", repo.Summaries())
	}
}

func TestWorkspaceSkillRepo_PlatformWins(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "shared")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\ndescription: disk\n---\n\ndisk body\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inner := stubSkillRepo{
		summaries: []trpcskill.Summary{{Name: "shared", Description: "platform"}},
		skills: map[string]*trpcskill.Skill{
			"shared": {Summary: trpcskill.Summary{Name: "shared", Description: "platform"}, Body: "platform body"},
		},
	}
	repo := &workspaceSkillRepo{inner: inner, cwd: root, roots: []string{root}}
	sk, err := repo.Get("shared")
	if err != nil {
		t.Fatal(err)
	}
	if sk.Body != "platform body" {
		t.Fatalf("platform must win, got %q", sk.Body)
	}
	if n := len(repo.Summaries()); n != 1 {
		t.Fatalf("summaries = %d, want 1 (no duplicate slug)", n)
	}
}

func TestReposkillsLookup(t *testing.T) {
	t.Parallel()
	e, ok := reposkills.Lookup([]reposkills.Entry{{Slug: "Foo", Name: "Foo Skill"}}, "foo")
	if !ok || e.Slug != "Foo" {
		t.Fatalf("lookup = %+v, %v", e, ok)
	}
}
