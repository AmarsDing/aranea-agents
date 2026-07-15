package service_test

import (
	"strings"
	"testing"
	"time"

	adminv1 "aranea-agents/api/kratos/admin/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestEncodePassword(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty string", ""},
		{"simple password", "password"},
		{"chinese password", "中文密码"},
		{"special chars", "p@ss!w0rd#"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.EncodePassword(tt.input)
			if tt.input == "" {
				if got != "" {
					t.Fatalf("empty input should produce empty hash, got %q", got)
				}
				return
			}
			// bcrypt hashes start with $2 and are ~60 chars long.
			if !strings.HasPrefix(got, "$2") {
				t.Fatalf("expected bcrypt hash prefix $2, got %q", got)
			}
			if len(got) < 50 {
				t.Fatalf("bcrypt hash too short: %d chars: %q", len(got), got)
			}
		})
	}
}

func TestEncodePassword_NotDeterministic(t *testing.T) {
	// bcrypt is salted: same input must produce different hashes.
	first := service.EncodePassword("test123")
	second := service.EncodePassword("test123")
	if first == second {
		t.Fatalf("bcrypt should be non-deterministic: first=%q second=%q", first, second)
	}
}

func TestEncodePassword_DifferentInputs(t *testing.T) {
	a := service.EncodePassword("alpha")
	b := service.EncodePassword("beta")
	if a == b {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestEncodePassword_VerifiableWithBcrypt(t *testing.T) {
	got := service.EncodePassword("hello")
	if err := bcrypt.CompareHashAndPassword([]byte(got), []byte("hello")); err != nil {
		t.Fatalf("bcrypt.CompareHashAndPassword failed: %v", err)
	}
}

func TestConvertAdmin(t *testing.T) {
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	m := &biz.Admin{
		ID:          1,
		Name:        "admin",
		Email:       "admin@example.com",
		Password:    "hashed",
		Access:      "superadmin",
		Avatar:      "https://avatar.example.com/1.png",
		WorkspaceID: "ws-tenant-a",
		CreateTime:  now,
		UpdateTime:  now,
	}
	pb := service.ConvertAdmin(m)
	if pb.Id != 1 || pb.Name != "admin" {
		t.Fatalf("id/name mismatch: id=%d name=%q", pb.Id, pb.Name)
	}
	if pb.Email != "admin@example.com" {
		t.Fatalf("email mismatch: %q", pb.Email)
	}
	if pb.Access != "superadmin" || pb.Avatar != "https://avatar.example.com/1.png" {
		t.Fatalf("access/avatar mismatch: access=%q avatar=%q", pb.Access, pb.Avatar)
	}
	if pb.WorkspaceId != "ws-tenant-a" {
		t.Fatalf("workspace_id mismatch: %q", pb.WorkspaceId)
	}
	if pb.CreateTime == nil || pb.UpdateTime == nil {
		t.Fatal("timestamps should not be nil")
	}
	if pb.CreateTime.AsTime().Unix() != now.Unix() {
		t.Fatalf("create_time mismatch: got %v, want %v", pb.CreateTime.AsTime(), now)
	}
	if pb.UpdateTime.AsTime().Unix() != now.Unix() {
		t.Fatalf("update_time mismatch: got %v, want %v", pb.UpdateTime.AsTime(), now)
	}
}

func TestConvertAdmin_PasswordNotExposed(t *testing.T) {
	m := &biz.Admin{
		ID:       1,
		Name:     "admin",
		Password: "secret_hash",
	}
	pb := service.ConvertAdmin(m)
	if pb.Password != "" {
		t.Fatalf("password should not be exposed in proto: %q", pb.Password)
	}
}

func TestConvertAdmin_EmptyAvatar(t *testing.T) {
	m := &biz.Admin{
		ID:     1,
		Name:   "admin",
		Avatar: "",
	}
	pb := service.ConvertAdmin(m)
	if pb.Avatar != "" {
		t.Fatalf("avatar should be empty: %q", pb.Avatar)
	}
}

func TestConvertAdmin_FieldsMapping(t *testing.T) {
	tests := []struct {
		name    string
		admin   *biz.Admin
		checkFn func(pb *adminv1.Admin) bool
		errMsg  string
	}{
		{
			name:    "id mapping",
			admin:   &biz.Admin{ID: 42, CreateTime: time.Now(), UpdateTime: time.Now()},
			checkFn: func(pb *adminv1.Admin) bool { return pb.Id == 42 },
			errMsg:  "id mismatch",
		},
		{
			name:    "name mapping",
			admin:   &biz.Admin{Name: "testuser", CreateTime: time.Now(), UpdateTime: time.Now()},
			checkFn: func(pb *adminv1.Admin) bool { return pb.Name == "testuser" },
			errMsg:  "name mismatch",
		},
		{
			name:    "email mapping",
			admin:   &biz.Admin{Email: "test@test.com", CreateTime: time.Now(), UpdateTime: time.Now()},
			checkFn: func(pb *adminv1.Admin) bool { return pb.Email == "test@test.com" },
			errMsg:  "email mismatch",
		},
		{
			name:    "workspace_id mapping",
			admin:   &biz.Admin{WorkspaceID: "ws-x", CreateTime: time.Now(), UpdateTime: time.Now()},
			checkFn: func(pb *adminv1.Admin) bool { return pb.WorkspaceId == "ws-x" },
			errMsg:  "workspace_id mismatch",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pb := service.ConvertAdmin(tt.admin)
			if !tt.checkFn(pb) {
				t.Fatalf(tt.errMsg+": %+v", pb)
			}
		})
	}
}
