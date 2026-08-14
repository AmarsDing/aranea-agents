package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	skillv1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/cli/client"
)

func TestListSkills(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/skills" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"sk-1","name":"Demo Skill"}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListSkills(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 skill, got %d", len(resp.Items))
	}
}

func TestGetSkill(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/skills/sk-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"skill":{"id":"sk-1","name":"Demo Skill"}}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.GetSkill(context.Background(), "sk-1")
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
}

func TestCreateSkill(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/skills" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"sk-new","name":"New Skill"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.CreateSkill(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
}

func TestUpdateSkill(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/skills/sk-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sk-1","name":"Updated Skill"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.UpdateSkill(context.Background(), "sk-1", nil)
	if err != nil {
		t.Fatalf("UpdateSkill: %v", err)
	}
}

func TestDeleteSkill(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/skills/sk-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteSkill(context.Background(), "sk-1"); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}
}

func TestToggleSkillEnabled_Enable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/skills/sk-1/enabled" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sk-1","enabled":true}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	skill, err := c.ToggleSkillEnabled(context.Background(), "sk-1", true)
	if err != nil {
		t.Fatalf("ToggleSkillEnabled: %v", err)
	}
	if !skill.Enabled {
		t.Error("expected skill.Enabled=true")
	}
}

func TestToggleSkillEnabled_Disable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/skills/sk-1/enabled" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sk-1","enabled":false}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.ToggleSkillEnabled(context.Background(), "sk-1", false)
	if err != nil {
		t.Fatalf("ToggleSkillEnabled(disable): %v", err)
	}
}

func TestPublishSkill(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/skills/sk-1/publish" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sk-1","status":"published"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	skill, err := c.PublishSkill(context.Background(), "sk-1")
	if err != nil {
		t.Fatalf("PublishSkill: %v", err)
	}
	if skill.Status != "published" {
		t.Errorf("expected status published, got %s", skill.Status)
	}
}

func TestListSkillFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/skills/sk-1/files" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"path":"SKILL.md","name":"SKILL.md","language":"markdown","size":128}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListSkillFiles(context.Background(), "sk-1")
	if err != nil {
		t.Fatalf("ListSkillFiles: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Path != "SKILL.md" {
		t.Errorf("unexpected items: %+v", resp.Items)
	}
}

func TestGetSkillFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/skills/sk-1/file" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("path"); got != "docs/guide.md" {
			t.Errorf("expected path=docs/guide.md, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"path":"docs/guide.md","content":"# Guide","language":"markdown"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.GetSkillFile(context.Background(), "sk-1", "docs/guide.md")
	if err != nil {
		t.Fatalf("GetSkillFile: %v", err)
	}
	if resp.Content != "# Guide" {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestUpdateSkillFile(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/skills/sk-1/file" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"path":"SKILL.md","content":"new","language":"markdown"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.UpdateSkillFile(context.Background(), "sk-1", "SKILL.md", "new")
	if err != nil {
		t.Fatalf("UpdateSkillFile: %v", err)
	}
	if resp.Path != "SKILL.md" {
		t.Errorf("unexpected path: %q", resp.Path)
	}
	if gotBody["path"] != "SKILL.md" || gotBody["content"] != "new" {
		t.Errorf("unexpected body: %v", gotBody)
	}
}

func TestDeleteSkillFile(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/skills/sk-1/files:delete" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteSkillFile(context.Background(), "sk-1", "old.md"); err != nil {
		t.Fatalf("DeleteSkillFile: %v", err)
	}
	if gotBody["path"] != "old.md" {
		t.Errorf("expected path=old.md in body, got %v", gotBody["path"])
	}
}

func TestImportSkillZip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/skills/import" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		var got struct {
			File     []byte `json:"file"`
			Filename string `json:"filename"`
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if got.Filename != "skills.zip" || string(got.File) != "zip-bytes" {
			t.Errorf("unexpected upload: name=%q body=%q", got.Filename, string(got.File))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jobId":"job-1"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ImportSkillZip(context.Background(), "skills.zip", []byte("zip-bytes"))
	if err != nil {
		t.Fatalf("ImportSkillZip: %v", err)
	}
	if resp.JobId != "job-1" {
		t.Errorf("expected job_id job-1, got %q", resp.JobId)
	}
}

func TestGetSkillImportJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/skills/import/job-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"job_id":"job-1","status":"completed","candidates":[{"candidate_id":"c1","name":"A","slug":"a","validation_status":"pass"}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	job, err := c.GetSkillImportJob(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("GetSkillImportJob: %v", err)
	}
	if job.Status != "completed" || len(job.Candidates) != 1 {
		t.Errorf("unexpected job: %+v", job)
	}
}

func TestApplySkillImport(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/skills/import/job-1/apply" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"created_skill_ids":["sk-1"],"message":"imported"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	decisions := []*skillv1.SkillImportDecision{{CandidateId: "c1", Action: "import_passed"}}
	result, err := c.ApplySkillImport(context.Background(), "job-1", decisions)
	if err != nil {
		t.Fatalf("ApplySkillImport: %v", err)
	}
	if len(result.CreatedSkillIds) != 1 || result.CreatedSkillIds[0] != "sk-1" {
		t.Errorf("unexpected result: %+v", result)
	}
	if gotBody["jobId"] != "job-1" {
		t.Errorf("expected jobId in body, got %v", gotBody["jobId"])
	}
}
