package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestGetFieldGuide(t *testing.T) {
	tests := []struct {
		name      string
		scope     biz.FieldScope
		fileName  string
		wantOk    bool
		wantTitle string
	}{
		{"category industry", biz.ScopeCategoryIndustry, "", true, "行业说明"},
		{"category department", biz.ScopeCategoryDepartment, "", true, "部门职责"},
		{"category position", biz.ScopeCategoryPosition, "", true, "岗位职责"},
		{"agent description", biz.ScopeAgentDescription, "", true, "Agent 描述（个体定位）"},
		{"agent file AGENTS_CORE", biz.ScopeAgentFile, "AGENTS_CORE.md", true, "核心角色说明"},
		{"agent file IDENTITY", biz.ScopeAgentFile, "IDENTITY.md", true, "身份与人设"},
		{"agent file RULE", biz.ScopeAgentFile, "RULE.md", true, "规则与合规"},
		{"spec extract", biz.ScopeSpecExtract, "", true, "组织结构抽取"},
		{"nonexistent scope", biz.FieldScope("nonexistent"), "", false, ""},
		{"nonexistent file", biz.ScopeAgentFile, "NONEXISTENT.md", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, ok := biz.GetFieldGuide(tt.scope, tt.fileName)
			if ok != tt.wantOk {
				t.Errorf("GetFieldGuide(%q,%q) ok = %v, want %v", tt.scope, tt.fileName, ok, tt.wantOk)
			}
			if ok && g.TitleZh != tt.wantTitle {
				t.Errorf("TitleZh = %q, want %q", g.TitleZh, tt.wantTitle)
			}
		})
	}
}

func TestListFieldGuides(t *testing.T) {
	guides := biz.ListFieldGuides()
	if len(guides) == 0 {
		t.Fatal("ListFieldGuides returned empty, expected at least 1")
	}
	seen := map[biz.FieldGuideKey]bool{}
	for _, g := range guides {
		k := biz.FieldGuideKey{Scope: g.Scope, FileName: g.FileName}
		if seen[k] {
			t.Errorf("duplicate guide key: %+v", k)
		}
		seen[k] = true
	}
}

func TestGetFieldGuidesForScope(t *testing.T) {
	tests := []struct {
		name       string
		scope      biz.FieldScope
		wantMinLen int
	}{
		{"category industry", biz.ScopeCategoryIndustry, 1},
		{"agent file", biz.ScopeAgentFile, 5},
		{"spec extract", biz.ScopeSpecExtract, 1},
		{"nonexistent", biz.FieldScope("nonexistent"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guides := biz.GetFieldGuidesForScope(tt.scope)
			if len(guides) < tt.wantMinLen {
				t.Errorf("GetFieldGuidesForScope(%q) returned %d guides, want at least %d", tt.scope, len(guides), tt.wantMinLen)
			}
			for _, g := range guides {
				if g.Scope != tt.scope {
					t.Errorf("guide scope = %q, want %q", g.Scope, tt.scope)
				}
			}
		})
	}
}

func TestFieldGuideBudget(t *testing.T) {
	g, ok := biz.GetFieldGuide(biz.ScopeCategoryIndustry, "")
	if !ok {
		t.Fatal("expected industry guide to exist")
	}
	if g.Budget.Soft <= 0 {
		t.Errorf("Soft budget = %d, want > 0", g.Budget.Soft)
	}
	if g.Budget.Hard <= 0 {
		t.Errorf("Hard budget = %d, want > 0", g.Budget.Hard)
	}
}

func TestFieldGuideKeyUniqueness(t *testing.T) {
	all := biz.ListFieldGuides()
	keys := map[biz.FieldGuideKey]bool{}
	for _, g := range all {
		k := biz.FieldGuideKey{Scope: g.Scope, FileName: g.FileName}
		if keys[k] {
			t.Errorf("duplicate key: scope=%q fileName=%q", k.Scope, k.FileName)
		}
		keys[k] = true
	}
}
