package service_test

import (
	"testing"

	v1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoSkill(t *testing.T) {
	avgDur := 150.5
	lastDur := 200

	tests := []struct {
		name  string
		input biz.Skill
		check func(t *testing.T, got *v1.Skill)
	}{
		{
			name: "full fields with version and tags",
			input: biz.Skill{
				ID:                   "skill-1",
				Name:                 "My Skill",
				Slug:                 "my-skill",
				Description:          "A test skill",
				ExtendsSkillID:       "",
				Status:               "published",
				Enabled:              true,
				FilesystemMissing:    false,
				SyncOrigin:           "manual",
				Visibility:           "public",
				DefaultConfigJSON:    "{}",
				InvokeCount:          10,
				SuccessCount:         8,
				FailureCount:         2,
				UsageCount7d:         5,
				LastAgentID:          "agent-1",
				LastAgentDisplayName: "Agent One",
				LastInvokedAt:        "2026-01-01T00:00:00Z",
				AvgDurationMS:        &avgDur,
				LastDurationMS:       &lastDur,
				CreatedAt:            "2026-01-01T00:00:00Z",
				UpdatedAt:            "2026-01-02T00:00:00Z",
				Tags: []biz.SkillTag{
					{Name: "utility", Source: "auto"},
					{Name: "test", Source: "manual"},
				},
				CurrentVersion: &biz.SkillVersionSummary{
					ID:               "ver-1",
					Version:          "1.0.0",
					ValidationStatus: "valid",
					PublishedAt:      "2026-01-01T00:00:00Z",
				},
				Permissions: biz.SkillPermissions{
					CanEdit:          true,
					CanDelete:        true,
					CanToggleEnabled: true,
					CanDuplicate:     true,
				},
			},
			check: func(t *testing.T, got *v1.Skill) {
				if got.Id != "skill-1" {
					t.Errorf("Id = %q, want %q", got.Id, "skill-1")
				}
				if got.Name != "My Skill" {
					t.Errorf("Name = %q, want %q", got.Name, "My Skill")
				}
				if got.Slug != "my-skill" {
					t.Errorf("Slug = %q, want %q", got.Slug, "my-skill")
				}
				if !got.Enabled {
					t.Errorf("Enabled = false, want true")
				}
				if got.InvokeCount != 10 {
					t.Errorf("InvokeCount = %d, want 10", got.InvokeCount)
				}
				if got.AvgDurationMs != 150.5 {
					t.Errorf("AvgDurationMs = %f, want 150.5", got.AvgDurationMs)
				}
				if got.LastDurationMs != 200 {
					t.Errorf("LastDurationMs = %d, want 200", got.LastDurationMs)
				}
				if len(got.Tags) != 2 {
					t.Fatalf("Tags len = %d, want 2", len(got.Tags))
				}
				if got.Tags[0].Name != "utility" || got.Tags[0].Source != "auto" {
					t.Errorf("Tags[0] = %v, unexpected", got.Tags[0])
				}
				if got.CurrentVersion == nil {
					t.Fatal("CurrentVersion = nil, want non-nil")
				}
				if got.CurrentVersion.Version != "1.0.0" {
					t.Errorf("CurrentVersion.Version = %q, want %q", got.CurrentVersion.Version, "1.0.0")
				}
				if got.Permissions == nil {
					t.Fatal("Permissions = nil, want non-nil")
				}
				if !got.Permissions.CanEdit {
					t.Errorf("Permissions.CanEdit = false, want true")
				}
			},
		},
		{
			name: "nil optional fields",
			input: biz.Skill{
				ID:      "skill-2",
				Name:    "Minimal",
				Slug:    "minimal",
				Status:  "draft",
				Enabled: false,
				Permissions: biz.SkillPermissions{
					CanEdit:          false,
					CanDelete:        false,
					CanToggleEnabled: false,
					CanDuplicate:     false,
				},
			},
			check: func(t *testing.T, got *v1.Skill) {
				if got.AvgDurationMs != 0 {
					t.Errorf("AvgDurationMs = %f, want 0", got.AvgDurationMs)
				}
				if got.LastDurationMs != 0 {
					t.Errorf("LastDurationMs = %d, want 0", got.LastDurationMs)
				}
				if len(got.Tags) != 0 {
					t.Errorf("Tags len = %d, want 0", len(got.Tags))
				}
				if got.CurrentVersion != nil {
					t.Errorf("CurrentVersion = %v, want nil", got.CurrentVersion)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoSkill(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoInvocation(t *testing.T) {
	tests := []struct {
		name  string
		input biz.SkillInvocation
		check func(t *testing.T, got *v1.SkillInvocation)
	}{
		{
			name: "full fields",
			input: biz.SkillInvocation{
				ID:               "inv-1",
				SkillID:          "skill-1",
				SkillName:        "My Skill",
				SkillVersion:     "1.0.0",
				AgentID:          "agent-1",
				AgentDisplayName: "Agent One",
				UserID:           "user-1",
				SessionID:        "sess-1",
				Status:           "success",
				DurationMS:       500,
				StartedAt:        "2026-01-01T00:00:00Z",
				EndedAt:          "2026-01-01T00:00:01Z",
				InputPreview:     "hello",
				InputHash:        "abc123",
				OutputPreview:    "hi there",
				ErrorCode:        "",
				ErrorMessage:     "",
				Source:           "runtime",
				ActivationID:     "act-1",
				MessageID:        "msg-1",
				Permissions: biz.SkillInvocationPermissions{
					CanViewDetail: true,
				},
			},
			check: func(t *testing.T, got *v1.SkillInvocation) {
				if got.Id != "inv-1" {
					t.Errorf("Id = %q, want %q", got.Id, "inv-1")
				}
				if got.SkillId != "skill-1" {
					t.Errorf("SkillId = %q, want %q", got.SkillId, "skill-1")
				}
				if got.DurationMs != 500 {
					t.Errorf("DurationMs = %d, want 500", got.DurationMs)
				}
				if got.Source != "runtime" {
					t.Errorf("Source = %q, want %q", got.Source, "runtime")
				}
				if got.Permissions == nil {
					t.Fatal("Permissions = nil, want non-nil")
				}
				if !got.Permissions.CanViewDetail {
					t.Errorf("Permissions.CanViewDetail = false, want true")
				}
			},
		},
		{
			name: "zero value",
			input: biz.SkillInvocation{
				Permissions: biz.SkillInvocationPermissions{},
			},
			check: func(t *testing.T, got *v1.SkillInvocation) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
				if got.DurationMs != 0 {
					t.Errorf("DurationMs = %d, want 0", got.DurationMs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoInvocation(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestToProtoVersionDetail(t *testing.T) {
	tests := []struct {
		name  string
		input biz.SkillVersionDetail
		check func(t *testing.T, got *v1.SkillVersionDetail)
	}{
		{
			name: "full fields",
			input: biz.SkillVersionDetail{
				ID:               "ver-1",
				SkillID:          "skill-1",
				Version:          "1.0.0",
				Status:           "published",
				ContentMarkdown:  "# My Skill\nHello",
				ValidationStatus: "valid",
				PublishedAt:      "2026-01-01T00:00:00Z",
				CreatedAt:        "2026-01-01T00:00:00Z",
				FileManifestJSON: "[]",
			},
			check: func(t *testing.T, got *v1.SkillVersionDetail) {
				if got.Id != "ver-1" {
					t.Errorf("Id = %q, want %q", got.Id, "ver-1")
				}
				if got.SkillId != "skill-1" {
					t.Errorf("SkillId = %q, want %q", got.SkillId, "skill-1")
				}
				if got.Version != "1.0.0" {
					t.Errorf("Version = %q, want %q", got.Version, "1.0.0")
				}
				if got.ValidationStatus != "valid" {
					t.Errorf("ValidationStatus = %q, want %q", got.ValidationStatus, "valid")
				}
				if got.FileManifestJson != "[]" {
					t.Errorf("FileManifestJson = %q, want %q", got.FileManifestJson, "[]")
				}
			},
		},
		{
			name:  "zero value",
			input: biz.SkillVersionDetail{},
			check: func(t *testing.T, got *v1.SkillVersionDetail) {
				if got.Id != "" {
					t.Errorf("Id = %q, want empty", got.Id)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoVersionDetail(tt.input)
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
