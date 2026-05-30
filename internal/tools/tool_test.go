package tools

import (
	"context"
	"testing"
)

func TestConfigString_firstKey(t *testing.T) {
	m := map[string]any{"api_key": "val"}
	got := ConfigString(m, "api_key", "key")
	if got != "val" {
		t.Fatalf("ConfigString = %q, want val", got)
	}
}

func TestConfigString_secondKey(t *testing.T) {
	m := map[string]any{"key": "val"}
	got := ConfigString(m, "api_key", "key")
	if got != "val" {
		t.Fatalf("ConfigString = %q, want val from second key", got)
	}
}

func TestConfigString_missingKey(t *testing.T) {
	m := map[string]any{}
	got := ConfigString(m, "api_key")
	if got != "" {
		t.Fatalf("ConfigString = %q, want empty", got)
	}
}

func TestConfigString_emptyValue(t *testing.T) {
	m := map[string]any{"api_key": ""}
	got := ConfigString(m, "api_key")
	if got != "" {
		t.Fatalf("ConfigString = %q, want empty for empty string", got)
	}
}

func TestConfigString_whitespaceValue(t *testing.T) {
	m := map[string]any{"api_key": "   "}
	got := ConfigString(m, "api_key")
	if got != "" {
		t.Fatalf("ConfigString = %q, want empty for whitespace-only string", got)
	}
}

func TestConfigString_trimmedValue(t *testing.T) {
	m := map[string]any{"api_key": "  val  "}
	got := ConfigString(m, "api_key")
	if got != "val" {
		t.Fatalf("ConfigString = %q, want val (trimmed)", got)
	}
}

func TestConfigString_nonStringValue(t *testing.T) {
	m := map[string]any{"api_key": 42}
	got := ConfigString(m, "api_key")
	if got != "" {
		t.Fatalf("ConfigString = %q, want empty for non-string value", got)
	}
}

func TestConfigString_nilMap(t *testing.T) {
	got := ConfigString(nil, "api_key")
	if got != "" {
		t.Fatalf("ConfigString = %q, want empty for nil map", got)
	}
}

func TestNewToolRegistration(t *testing.T) {
	factory := func(ctx context.Context) (Tool, error) { return nil, nil }
	reg := NewToolRegistration("test", "desc", factory)
	if reg.Name != "test" {
		t.Fatalf("Name = %q, want test", reg.Name)
	}
	if reg.Description != "desc" {
		t.Fatalf("Description = %q, want desc", reg.Description)
	}
	if reg.Factory == nil {
		t.Fatal("Factory should not be nil")
	}
	if reg.ToolSetFactory != nil {
		t.Fatal("ToolSetFactory should be nil for tool registration")
	}
	if reg.EnabledByDefault {
		t.Fatal("EnabledByDefault should be false by default")
	}
}

func TestNewToolSetRegistration(t *testing.T) {
	factory := func(ctx context.Context) (ToolSet, error) { return nil, nil }
	reg := NewToolSetRegistration("test_set", "desc", factory)
	if reg.Name != "test_set" {
		t.Fatalf("Name = %q, want test_set", reg.Name)
	}
	if reg.Description != "desc" {
		t.Fatalf("Description = %q, want desc", reg.Description)
	}
	if reg.ToolSetFactory == nil {
		t.Fatal("ToolSetFactory should not be nil")
	}
	if reg.Factory != nil {
		t.Fatal("Factory should be nil for toolset registration")
	}
	if reg.EnabledByDefault {
		t.Fatal("EnabledByDefault should be false by default")
	}
}

func TestRegistryByTag_found(t *testing.T) {
	regs := RegistryByTag("filesystem")
	if len(regs) == 0 {
		t.Fatal("expected at least one registration with filesystem tag")
	}
	for _, r := range regs {
		found := false
		for _, tag := range r.Tags {
			if tag == "filesystem" {
				found = true
			}
		}
		if !found {
			t.Fatalf("registration %q does not have filesystem tag", r.Name)
		}
	}
}

func TestRegistryByTag_notFound(t *testing.T) {
	regs := RegistryByTag("nonexistent_tag_xyz")
	if len(regs) != 0 {
		t.Fatalf("expected 0 registrations, got %d", len(regs))
	}
}

func TestRegistryByTag_empty(t *testing.T) {
	regs := RegistryByTag("")
	if regs != nil {
		t.Fatalf("expected nil for empty tag, got %v", regs)
	}
}

func TestRegistryByTag_caseInsensitive(t *testing.T) {
	regs := RegistryByTag("FILESYSTEM")
	if len(regs) == 0 {
		t.Fatal("expected case-insensitive match for FILESYSTEM")
	}
}

func TestRegistryByCategory_found(t *testing.T) {
	regs := RegistryByCategory("search")
	if len(regs) == 0 {
		t.Fatal("expected at least one registration with search category")
	}
	for _, r := range regs {
		if r.Category != "search" {
			t.Fatalf("registration %q has category %q, want search", r.Name, r.Category)
		}
	}
}

func TestRegistryByCategory_notFound(t *testing.T) {
	regs := RegistryByCategory("nonexistent_cat_xyz")
	if len(regs) != 0 {
		t.Fatalf("expected 0 registrations, got %d", len(regs))
	}
}

func TestRegistryByCategory_empty(t *testing.T) {
	regs := RegistryByCategory("")
	if regs != nil {
		t.Fatalf("expected nil for empty category, got %v", regs)
	}
}

func TestRegistryByCategory_caseInsensitive(t *testing.T) {
	regs := RegistryByCategory("SEARCH")
	if len(regs) == 0 {
		t.Fatal("expected case-insensitive match for SEARCH")
	}
}
