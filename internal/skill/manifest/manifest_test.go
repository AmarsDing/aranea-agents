package manifest

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestParse_ValidManifest(t *testing.T) {
	input := `---
name: "My Skill"
description: "A test skill"
---
# My Skill Body
Do something useful.`
	m := Parse(input)
	if m.Name != "My Skill" {
		t.Errorf("Name = %q, want %q", m.Name, "My Skill")
	}
	if m.Description != "A test skill" {
		t.Errorf("Description = %q, want %q", m.Description, "A test skill")
	}
	if m.Body == "" {
		t.Error("Body should not be empty")
	}
}

// P-r4-BOM：PowerShell/Windows 编辑器写出的 SKILL.md 常带 UTF-8 BOM（U+FEFF），
// Parse 必须剔除 BOM 后再识别 frontmatter，否则整个 frontmatter 失效、
// description 退化为 "---" 字符串，污染相似度判定。
func TestParse_UTF8BOM(t *testing.T) {
	input := "\ufeff" + `---
name: "BOM Skill"
description: "Has a byte-order mark"
tags: [productivity, standup]
---
# BOM Skill
Body here.`
	m := Parse(input)
	if m.Name != "BOM Skill" {
		t.Errorf("Name = %q, want %q", m.Name, "BOM Skill")
	}
	if m.Description != "Has a byte-order mark" {
		t.Errorf("Description = %q, want %q", m.Description, "Has a byte-order mark")
	}
	if len(m.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(m.Tags))
	}
	if m.Body == "" || m.Body == input {
		t.Errorf("Body should be frontmatter-stripped content, got %q", m.Body)
	}
}

func TestParse_EmptyInput(t *testing.T) {
	m := Parse("")
	if m.Name != "" {
		t.Errorf("Name = %q, want empty", m.Name)
	}
	if m.Description != "" {
		t.Errorf("Description = %q, want empty", m.Description)
	}
	if len(m.Tags) != 0 {
		t.Errorf("Tags = %v, want empty", m.Tags)
	}
	if len(m.Triggers) != 0 {
		t.Errorf("Triggers = %v, want empty", m.Triggers)
	}
	if len(m.Tools) != 0 {
		t.Errorf("Tools = %v, want empty", m.Tools)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	input := `---
: : :
name: "Still Works"
---
Body here.`
	m := Parse(input)
	if m.Name != "Still Works" {
		t.Errorf("Name = %q, want %q", m.Name, "Still Works")
	}
}

func TestParse_MissingName(t *testing.T) {
	input := `---
description: "No name here"
---
Some body.`
	m := Parse(input)
	if m.Name != "" {
		t.Errorf("Name = %q, want empty when name not in frontmatter", m.Name)
	}
	if m.Description != "No name here" {
		t.Errorf("Description = %q, want %q", m.Description, "No name here")
	}
}

func TestParse_WithTags(t *testing.T) {
	input := `---
name: "Tagged Skill"
tags: [analytics, nlp, sales]
---
Body.`
	m := Parse(input)
	if len(m.Tags) != 3 {
		t.Fatalf("Tags len = %d, want 3", len(m.Tags))
	}
	want := map[string]bool{"analytics": true, "nlp": true, "sales": true}
	for _, tag := range m.Tags {
		if !want[tag.Name] {
			t.Errorf("unexpected tag %q", tag.Name)
		}
		if tag.Source != "user" {
			t.Errorf("tag %q source = %q, want %q", tag.Name, tag.Source, "user")
		}
	}
}

func TestParse_WithTaxonomyPaths(t *testing.T) {
	input := `---
name: "Taxonomy Skill"
description: "Skill with taxonomy paths"
taxonomy_paths: ["分析与推理/自然语言理解（情感分析）", "数据获取与集成/内部数据源/文件系统读取（读取表格）"]
---
Body content.`
	m := Parse(input)
	if m.Name != "Taxonomy Skill" {
		t.Errorf("Name = %q, want %q", m.Name, "Taxonomy Skill")
	}
	if m.Description != "Skill with taxonomy paths" {
		t.Errorf("Description = %q, want %q", m.Description, "Skill with taxonomy paths")
	}
	if m.Body == "" {
		t.Error("Body should not be empty")
	}
	_ = biz.SkillTag{}
}

// YAML block scalar：alibabacloud 系列 SKILL.md 用 `description: |` / `>` 书写
// 多行描述，Parse 必须消费后续缩进行，否则 description 退化为字面量 "|"/">"。
func TestParse_DescriptionBlockScalarLiteral(t *testing.T) {
	input := `---
name: alibabacloud-rds-copilot
description: |
  RDS Copilot helps diagnose databases.
  It covers SQL optimization and troubleshooting.
tags: [ops, cloud]
---
# Body
Content.`
	m := Parse(input)
	want := "RDS Copilot helps diagnose databases.\nIt covers SQL optimization and troubleshooting."
	if m.Description != want {
		t.Errorf("Description = %q, want %q", m.Description, want)
	}
	// block scalar 后的键仍须解析
	if len(m.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2 (keys after block scalar must still parse)", len(m.Tags))
	}
	if !strings.Contains(m.Body, "# Body") {
		t.Errorf("Body = %q, want frontmatter-stripped body", m.Body)
	}
}

func TestParse_DescriptionBlockScalarFolded(t *testing.T) {
	input := `---
name: alibabacloud-find-skills
description: >
  Find skills across catalogs.
  Folds lines into one paragraph.
---
Body.`
	m := Parse(input)
	want := "Find skills across catalogs. Folds lines into one paragraph."
	if m.Description != want {
		t.Errorf("Description = %q, want %q", m.Description, want)
	}
}

func TestParse_DescriptionBlockScalarChomping(t *testing.T) {
	input := `---
name: claude-api
description: |-
  Build with the Claude API.
---
Body.`
	m := Parse(input)
	if m.Description != "Build with the Claude API." {
		t.Errorf("Description = %q, want %q", m.Description, "Build with the Claude API.")
	}
}
