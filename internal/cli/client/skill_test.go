package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
