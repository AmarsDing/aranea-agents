// Package main implements a conversion tool that transforms agency-agents
// markdown files into aranea-agents Pack directory structure.
//
// Usage:
//
//	go run ./cmd/tools/agency-converter \
//	  -src F:\agency-agents-main \
//	  -dst internal/scenario/packs/agency-pack \
//	  -zh  F:\agency-agents-main\scripts\i18n\agent-names-zh.json
//
// The tool walks all division directories under -src, parses each agent .md
// file (YAML frontmatter + markdown body), splits the body into sections,
// classifies each agent into a company/department, and generates the complete
// Pack directory structure including manifest.yaml, taxonomy.yaml, agent yaml
// configs, and prompt file stubs (English originals — Chinese translation is
// handled separately in Phase 3b).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// -----------------------------------------------------------------------------
// Data structures
// -----------------------------------------------------------------------------

// agentFrontmatter represents the YAML frontmatter of an agency-agents .md file.
type agentFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Color       string `yaml:"color"`
	Emoji       string `yaml:"emoji"`
	Vibe        string `yaml:"vibe"`
	Tools       string `yaml:"tools"` // comma-separated, optional
}

// zhTranslation represents one entry in agent-names-zh.json.
type zhTranslation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// section represents a parsed markdown section (## header + body).
type section struct {
	Header string // raw header line without the ## prefix
	Body   string // body content (trimmed)
}

// departmentInfo describes a target department in the Pack taxonomy.
type departmentInfo struct {
	CompanyKey   string
	DeptKey      string
	DeptName     string
	DeptDesc     string
	CompanySort  int
	DeptSort     int
}

// promptFileSpec describes a generated prompt file.
type promptFileSpec struct {
	Name string // e.g. IDENTITY.md
	Body string // file content
}

// agentRecord holds all parsed data for a single agent.
type agentRecord struct {
	FileBasename string // e.g. engineering-frontend-developer
	Division     string // e.g. engineering
	Frontmatter  agentFrontmatter
	Sections     []section
	IntroPara    string // paragraph(s) between H1 and first ##

	// Resolved mapping
	Dept       departmentInfo
	PositionKey string // e.g. frontend_developer
	AgentKey    string // e.g. frontend_developer__general

	// Chinese translation (if available)
	ZhName        string
	ZhDescription string

	// Generated prompt files
	PromptFiles []promptFileSpec
}

// -----------------------------------------------------------------------------
// Division → Company/Department mapping
// -----------------------------------------------------------------------------

// companyDefs defines the 3 target companies.
var companyDefs = []struct {
	Key         string
	Name        string
	Description string
	Icon        string
}{
	{"digital_content_media", "数字内容与媒体传播公司",
		"创意构思→内容创作→视觉设计→媒体发布→付费推广→销售转化→财务→客服的完整业务闭环", "megaphone"},
	{"digital_tech", "软件工程与数字科技产品公司",
		"产品规划→项目管理→架构设计→开发实现→测试→部署运维→安全保障→合规审计的完整工程闭环", "code"},
	{"healthcare", "医疗公司",
		"临床证据→医疗创新→主权健康系统的专业医疗服务", "heart-pulse"},
}

// classifyAgent determines which company/department an agent belongs to based
// on its division and filename. Returns departmentInfo and the position key
// (extracted from filename).
func classifyAgent(division, fileBasename string) (departmentInfo, string) {
	// Strip the "{division}-" prefix from the filename to get the agent slug.
	// e.g. "engineering-frontend-developer" → "frontend-developer"
	slug := strings.TrimPrefix(fileBasename, division+"-")

	// Convert slug to snake_case position key: "frontend-developer" → "frontend_developer"
	posKey := slugToSnakeCase(slug)

	// Default: division maps 1:1 to a department.
	// Special handling for engineering, marketing, specialized, healthcare.
	switch division {
	case "engineering":
		return classifyEngineering(slug, posKey)
	case "marketing":
		return classifyMarketing(slug, posKey)
	case "specialized":
		return classifySpecialized(slug, posKey)
	case "healthcare":
		return classifyHealthcare(slug, posKey)
	default:
		return simpleDept(division, posKey)
	}
}

// simpleDept handles the 1:1 division→department mapping for most divisions.
func simpleDept(division, posKey string) (departmentInfo, string) {
	m := divisionToDept(division)
	return m, posKey
}

// divisionToDept maps a division name to its departmentInfo (for 1:1 cases).
func divisionToDept(division string) departmentInfo {
	switch division {
	case "academic":
		return departmentInfo{"digital_content_media", "creative_planning", "创意策划部",
			"人类学、地理学、历史学、叙事学、心理学、统计学等学术研究支撑创意策划", 1, 1}
	case "design":
		return departmentInfo{"digital_content_media", "brand_design", "品牌设计部",
			"UI/UX设计、品牌守护、视觉叙事等品牌视觉与体验设计", 1, 2}
	case "paid-media":
		return departmentInfo{"digital_content_media", "paid_promotion", "付费推广部",
			"PPC策略、搜索词分析、程序化购买、付费社交等付费媒体推广", 1, 5}
	case "sales":
		return departmentInfo{"digital_content_media", "sales_dept", "销售部",
			"外呼策略、商机策略、售前工程、提案策略等销售与客户策略", 1, 7}
	case "finance":
		return departmentInfo{"digital_content_media", "finance_dept", "财务部",
			"财务追踪、财务分析、FP&A、税务策略、CFO等财务管理", 1, 8}
	case "support":
		return departmentInfo{"digital_content_media", "customer_support", "客户支持部",
			"客户支持、数据分析、基础设施维护等客户支持与运营", 1, 9}
	case "product":
		return departmentInfo{"digital_tech", "product_dept", "产品部",
			"产品经理、趋势研究、反馈综合等产品规划与管理", 2, 1}
	case "project-management":
		return departmentInfo{"digital_tech", "project_management", "项目管理部",
			"制片人、项目协调、高级项目经理等项目全生命周期管理", 2, 2}
	case "game-development":
		return departmentInfo{"digital_tech", "game_dev", "游戏开发部",
			"游戏设计、Unity/Unreal/Godot/Blender/Roblox等游戏引擎开发", 2, 6}
	case "spatial-computing":
		return departmentInfo{"digital_tech", "spatial_computing", "空间计算部",
			"XR架构、visionOS、macOS Metal等空间计算与沉浸式体验开发", 2, 7}
	case "testing":
		return departmentInfo{"digital_tech", "quality_assurance", "质量保障部",
			"测试自动化、性能基准、API测试、无障碍审计等质量保障", 2, 8}
	case "security":
		return departmentInfo{"digital_tech", "security_dept", "安全部",
			"安全架构、渗透测试、云安全、威胁检测等信息安全", 2, 11}
	case "gis":
		return departmentInfo{"digital_tech", "gis_solutions", "GIS解决方案部",
			"GIS分析、空间数据工程、GeoAI等地理信息系统解决方案", 2, 13}
	default:
		// Fallback: put unknown divisions into company 1 special services
		return departmentInfo{"digital_content_media", "special_services", "专项服务部",
			"跨领域专项服务与专业支持", 1, 10}
	}
}

// classifyEngineering splits the engineering division into 5 departments.
func classifyEngineering(slug, posKey string) (departmentInfo, string) {
	// Keywords for each sub-department
	frontend := []string{"frontend", "desktop-app", "webassembly", "wechat-mini-program",
		"uswds", "section-508", "i18n"}
	mobile := []string{"mobile", "voice-ai"}
	ops := []string{"sre", "devops", "network", "incident-response", "it-service",
		"finops", "identity-access", "codebase-onboarding"}
	architecture := []string{"software-architect", "code-reviewer", "technical-writer",
		"git-workflow", "minimal-change", "senior-developer", "rapid-prototyper"}

	if containsAny(slug, frontend) {
		return departmentInfo{"digital_tech", "frontend_dev", "前端开发部",
			"前端、桌面应用、WebAssembly、小程序等前端与客户端开发", 2, 4}, posKey
	}
	if containsAny(slug, mobile) {
		return departmentInfo{"digital_tech", "mobile_dev", "移动开发部",
			"移动应用构建、移动发布、语音AI集成等移动端开发", 2, 5}, posKey
	}
	if containsAny(slug, ops) {
		return departmentInfo{"digital_tech", "ops", "运维部",
			"SRE、DevOps自动化、网络工程、故障响应、IT服务等运维与可靠性", 2, 9}, posKey
	}
	if containsAny(slug, architecture) {
		return departmentInfo{"digital_tech", "architecture", "架构部",
			"软件架构、代码审查、技术文档、Git工作流等架构与工程规范", 2, 10}, posKey
	}
	// Default: backend
	return departmentInfo{"digital_tech", "backend_dev", "后端开发部",
		"后端架构、API平台、数据库、数据工程、CMS、搜索、支付、AI等后端开发", 2, 3}, posKey
}

// classifyMarketing splits the marketing division into 3 departments.
func classifyMarketing(slug, posKey string) (departmentInfo, string) {
	// E-commerce keywords
	ecommerce := []string{"china-ecommerce", "cross-border-ecommerce", "livestream-commerce",
		"carousel-growth"}
	// Platform operations keywords (social media platforms)
	platformOps := []string{"seo", "baidu", "bilibili", "douyin", "instagram", "kuaishou",
		"linkedin", "reddit", "social-media", "tiktok", "twitter", "wechat-official",
		"weibo", "x-twitter", "xiaohongshu", "zhihu", "app-store", "short-video",
		"growth-hacker"}

	if containsAny(slug, ecommerce) {
		return departmentInfo{"digital_content_media", "cross_border_ecommerce", "跨境电商部",
			"中国电商运营、跨境电商、直播带货等电商运营", 1, 6}, posKey
	}
	if containsAny(slug, platformOps) {
		return departmentInfo{"digital_content_media", "media_operations", "媒体运营部",
			"SEO、小红书/B站/抖音/知乎/微博/快手等平台运营与社交媒体策略", 1, 4}, posKey
	}
	// Default: content creation
	return departmentInfo{"digital_content_media", "content_creation", "内容创作部",
		"内容创作、图书联合、播客、PR传播、邮件策略等多平台内容创作", 1, 3}, posKey
}

// classifySpecialized splits the specialized division into compliance vs non-compliance.
func classifySpecialized(slug, posKey string) (departmentInfo, string) {
	compliance := []string{"data-privacy-officer", "esg-sustainability", "fedramp-rmf"}
	if containsAny(slug, compliance) {
		return departmentInfo{"digital_tech", "compliance_audit", "合规审计部",
			"数据隐私、ESG可持续、FedRAMP合规等合规与审计", 2, 12}, posKey
	}
	return departmentInfo{"digital_content_media", "special_services", "专项服务部",
		"参谋长、业务策略、变革管理、客户成功、运营、HR、法务、供应链等跨领域专项服务", 1, 10}, posKey
}

// classifyHealthcare maps each healthcare agent to its own department.
func classifyHealthcare(slug, posKey string) (departmentInfo, string) {
	switch {
	case strings.Contains(slug, "clinical-evidence"):
		return departmentInfo{"healthcare", "clinical_evidence", "临床证据部",
			"临床证据agent，提供循证医学证据支撑", 3, 1}, posKey
	case strings.Contains(slug, "innovation"):
		return departmentInfo{"healthcare", "medical_innovation", "医疗创新部",
			"医疗创新策略师，推动医疗技术与模式创新", 3, 2}, posKey
	case strings.Contains(slug, "sovereign-health"):
		return departmentInfo{"healthcare", "sovereign_health", "主权健康部",
			"主权健康系统agent，支撑国家健康体系建设", 3, 3}, posKey
	default:
		return departmentInfo{"healthcare", "clinical_evidence", "临床证据部",
			"临床证据与医疗研究", 3, 1}, posKey
	}
}

// -----------------------------------------------------------------------------
// Markdown parsing
// -----------------------------------------------------------------------------

// parseFrontmatter splits a .md file into YAML frontmatter and markdown body.
func parseFrontmatter(content string) (agentFrontmatter, string, error) {
	var fm agentFrontmatter

	// Check for --- delimiter
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return fm, content, nil // no frontmatter
	}

	// Find closing ---
	rest := content[4:] // skip opening "---\n"
	if strings.HasPrefix(content, "---\r\n") {
		rest = content[5:]
	}
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return fm, content, fmt.Errorf("frontmatter not closed")
	}

	fmText := rest[:endIdx]
	bodyStart := endIdx + 4 // skip "\n---"
	if bodyStart < len(rest) {
		// skip the newline after ---
		if rest[bodyStart] == '\n' {
			bodyStart++
		} else if bodyStart+1 < len(rest) && rest[bodyStart] == '\r' && rest[bodyStart+1] == '\n' {
			bodyStart += 2
		}
	}
	body := rest[bodyStart:]

	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return fm, body, fmt.Errorf("parse frontmatter: %w", err)
	}
	return fm, body, nil
}

// splitSections splits markdown body into sections by ## headers.
// Returns the intro paragraph (text before first ##) and a slice of sections.
func splitSections(body string) (string, []section) {
	lines := strings.Split(body, "\n")
	var introLines []string
	var sections []section
	var currentHeader string
	var currentBody []string
	inSection := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			// Save previous section
			if inSection {
				sections = append(sections, section{
					Header: currentHeader,
					Body:   strings.TrimSpace(strings.Join(currentBody, "\n")),
				})
			}
			currentHeader = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			currentBody = nil
			inSection = true
		} else if strings.HasPrefix(line, "# ") {
			// H1 title — skip, it's part of intro
			if !inSection {
				introLines = append(introLines, line)
			} else {
				currentBody = append(currentBody, line)
			}
		} else if inSection {
			currentBody = append(currentBody, line)
		} else {
			introLines = append(introLines, line)
		}
	}
	// Save last section
	if inSection {
		sections = append(sections, section{
			Header: currentHeader,
			Body:   strings.TrimSpace(strings.Join(currentBody, "\n")),
		})
	}

	intro := strings.TrimSpace(strings.Join(introLines, "\n"))
	return intro, sections
}

// -----------------------------------------------------------------------------
// Section → Prompt file mapping
// -----------------------------------------------------------------------------

// classifySection determines which prompt file a section belongs to.
// Returns the target filename (e.g. "IDENTITY.md") and whether it's a primary
// match (true) or a secondary merge target (false).
func classifySection(header string) (string, bool) {
	h := strings.ToLower(header)

	// IDENTITY: "Identity & Memory", "Identity & Role Definition", "Identity"
	if strings.Contains(h, "identity") || strings.Contains(h, "role definition") {
		return "IDENTITY.md", true
	}
	// MISSION: "Core Mission", "Core Capabilities" (when no Advanced Capabilities)
	if strings.Contains(h, "core mission") {
		return "MISSION.md", true
	}
	// RULE: "Critical Rules", "Decision Framework"
	if strings.Contains(h, "critical rule") || strings.Contains(h, "decision framework") {
		return "RULE.md", true
	}
	// CAPABILITIES: "Advanced Capabilities", "Specialized Skills"
	if strings.Contains(h, "advanced capabilit") || strings.Contains(h, "specialized skill") {
		return "CAPABILITIES.md", true
	}
	// WORKFLOW: "Workflow Process", "Workflow"
	if strings.Contains(h, "workflow") {
		return "WORKFLOW.md", true
	}
	// DELIVERABLES: "Technical Deliverables", "Deliverable Template"
	if strings.Contains(h, "technical deliverable") || strings.Contains(h, "deliverable template") {
		return "DELIVERABLES.md", true
	}
	// COMMUNICATION: "Communication Style", "Learning & Memory", "Success Metrics"
	if strings.Contains(h, "communication") || strings.Contains(h, "learning") ||
		strings.Contains(h, "success metric") || strings.Contains(h, "learning and memory") ||
		strings.Contains(h, "learning & memory") {
		return "COMMUNICATION.md", false
	}
	// Core Capabilities (when no Advanced Capabilities exists) → MISSION fallback
	if strings.Contains(h, "core capabilit") {
		return "MISSION.md", false
	}

	return "", false
}

// generatePromptFiles maps sections to prompt files and generates their content.
func generatePromptFiles(rec *agentRecord) {
	// Collect sections by target file
	fileSections := make(map[string][]section)
	var fileOrder []string

	// If there's an intro paragraph (text between H1 and first ##),
	// prepend it to IDENTITY.md. Use Header="" as a marker to avoid
	// double-wrapping with "## " prefix.
	introAdded := false

	for _, sec := range rec.Sections {
		target, _ := classifySection(sec.Header)
		if target == "" {
			continue // skip unmapped sections
		}
		if _, exists := fileSections[target]; !exists {
			fileOrder = append(fileOrder, target)
		}
		// For IDENTITY.md, prepend intro paragraph if available
		if target == "IDENTITY.md" && !introAdded && rec.IntroPara != "" {
			// Store as a section with Header="" so the wrapper won't add
			// "## " prefix. The full content (intro + section header + body)
			// is stored in Body.
			fullContent := rec.IntroPara + "\n\n## " + sec.Header + "\n" + sec.Body
			fileSections[target] = append(fileSections[target], section{
				Header: "", // marker: don't wrap
				Body:   fullContent,
			})
			introAdded = true
		} else {
			fileSections[target] = append(fileSections[target], sec)
		}
	}

	// If no IDENTITY.md was created but we have an intro paragraph, create one
	if !introAdded && rec.IntroPara != "" {
		fileSections["IDENTITY.md"] = []section{{
			Header: "", // marker: don't wrap
			Body:   rec.IntroPara,
		}}
		fileOrder = append([]string{"IDENTITY.md"}, fileOrder...)
	}

	// Generate file contents
	sort.Strings(fileOrder) // alphabetical for deterministic output
	for _, name := range fileOrder {
		sections := fileSections[name]
		var parts []string
		for _, s := range sections {
			if s.Header == "" {
				// No header wrapping (intro content or pre-formatted)
				parts = append(parts, s.Body)
			} else {
				parts = append(parts, "## "+s.Header+"\n"+s.Body)
			}
		}
		body := strings.Join(parts, "\n\n")
		if body == "" {
			continue
		}
		rec.PromptFiles = append(rec.PromptFiles, promptFileSpec{
			Name: name,
			Body: body + "\n",
		})
	}
}

// -----------------------------------------------------------------------------
// Utility functions
// -----------------------------------------------------------------------------

// slugToSnakeCase converts a hyphenated slug to snake_case.
// "frontend-developer" → "frontend_developer"
func slugToSnakeCase(slug string) string {
	return strings.ReplaceAll(slug, "-", "_")
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// nonDivisionDirs are top-level directories that are NOT agent divisions.
var nonDivisionDirs = map[string]bool{
	"examples":      true,
	"integrations":  true,
	"scripts":       true,
	"strategy":      true,
	".github":       true,
	".git":          true,
	"node_modules":  true,
}

// -----------------------------------------------------------------------------
// Pack generation
// -----------------------------------------------------------------------------

// generateManifestYAML creates the manifest.yaml content.
func generateManifestYAML() string {
	return `api_version: v1
kind: industry
name: agency-pack
description: The Agency 230+ AI agent 模板库导入 Pack（3 公司 / 多部门 / 全岗位）
version: "1.0.0"
author: aranea-agents
created_at: "2026-07-14"
contents:
  organization: true
`
}

// generateTaxonomyYAML creates the taxonomy.yaml content from the list of agents.
func generateTaxonomyYAML(records []agentRecord) string {
	// Group agents by company → department
	type deptGroup struct {
		Info    departmentInfo
		Agents  []*agentRecord
	}
	type companyGroup struct {
		Key      string
		Name     string
		Desc     string
		Icon     string
		Sort     int
		Depts    map[string]*deptGroup
		DeptKeys []string // ordered
	}

	companies := make(map[string]*companyGroup)
	var companyKeys []string

	for i := range records {
		rec := &records[i]
		ck := rec.Dept.CompanyKey
		dk := rec.Dept.DeptKey

		if _, exists := companies[ck]; !exists {
			// Find company def
			for _, cd := range companyDefs {
				if cd.Key == ck {
					companies[ck] = &companyGroup{
						Key: cd.Key, Name: cd.Name, Desc: cd.Description,
						Icon: cd.Icon, Sort: rec.Dept.CompanySort,
						Depts: make(map[string]*deptGroup),
					}
					companyKeys = append(companyKeys, ck)
					break
				}
			}
		}
		cg := companies[ck]
		if cg == nil {
			continue
		}

		if _, exists := cg.Depts[dk]; !exists {
			cg.Depts[dk] = &deptGroup{Info: rec.Dept}
			cg.DeptKeys = append(cg.DeptKeys, dk)
		}
		cg.Depts[dk].Agents = append(cg.Depts[dk].Agents, rec)
	}

	// Sort company keys by sort order
	sort.Slice(companyKeys, func(i, j int) bool {
		return companies[companyKeys[i]].Sort < companies[companyKeys[j]].Sort
	})

	// Build YAML manually for control over ordering
	var b strings.Builder
	b.WriteString("companies:\n")

	for _, ck := range companyKeys {
		cg := companies[ck]
		b.WriteString(fmt.Sprintf("- key: %s\n", cg.Key))
		b.WriteString(fmt.Sprintf("  name: %s\n", cg.Name))
		b.WriteString(fmt.Sprintf("  icon: %s\n", cg.Icon))
		b.WriteString(fmt.Sprintf("  description: %s\n", cg.Desc))
		b.WriteString(fmt.Sprintf("  sort_order: %d\n", cg.Sort))
		b.WriteString("  departments:\n")

		// Sort departments by sort order
		sort.Slice(cg.DeptKeys, func(i, j int) bool {
			return cg.Depts[cg.DeptKeys[i]].Info.DeptSort < cg.Depts[cg.DeptKeys[j]].Info.DeptSort
		})

		for _, dk := range cg.DeptKeys {
			dg := cg.Depts[dk]
			b.WriteString(fmt.Sprintf("  - key: %s\n", dg.Info.DeptKey))
			b.WriteString(fmt.Sprintf("    name: %s\n", dg.Info.DeptName))
			b.WriteString(fmt.Sprintf("    description: %s\n", dg.Info.DeptDesc))
			b.WriteString(fmt.Sprintf("    sort_order: %d\n", dg.Info.DeptSort))
			b.WriteString("    positions:\n")

			// Sort agents by position key
			sort.Slice(dg.Agents, func(i, j int) bool {
				return dg.Agents[i].PositionKey < dg.Agents[j].PositionKey
			})

			for idx, rec := range dg.Agents {
				name := rec.ZhName
				if name == "" {
					name = rec.Frontmatter.Name
				}
				desc := rec.ZhDescription
				if desc == "" {
					desc = rec.Frontmatter.Description
				}
				b.WriteString(fmt.Sprintf("    - key: %s\n", rec.PositionKey))
				b.WriteString(fmt.Sprintf("      name: %s\n", yamlQuote(name)))
				b.WriteString(fmt.Sprintf("      description: %s\n", yamlQuote(desc)))
				b.WriteString(fmt.Sprintf("      sort_order: %d\n", idx+1))
				b.WriteString("      seniority_level: senior\n")
				b.WriteString(fmt.Sprintf("      variants:\n      - key: general\n        name: 通用\n"))
			}
		}
	}

	return b.String()
}

// generateAgentYAML creates a single agent yaml config.
func generateAgentYAML(rec *agentRecord) string {
	name := rec.ZhName
	if name == "" {
		name = rec.Frontmatter.Name
	}
	desc := rec.ZhDescription
	if desc == "" {
		desc = rec.Frontmatter.Description
	}
	emoji := rec.Frontmatter.Emoji
	if emoji == "" {
		emoji = "🤖"
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("key: %s\n", rec.AgentKey))
	b.WriteString(fmt.Sprintf("display_name: %s\n", yamlQuote(name)))
	b.WriteString(fmt.Sprintf("description: %s\n", yamlQuote(desc)))
	b.WriteString(fmt.Sprintf("icon: %s\n", emoji))
	b.WriteString(fmt.Sprintf("position_key: %s/%s/%s\n",
		rec.Dept.CompanyKey, rec.Dept.DeptKey, rec.PositionKey))
	b.WriteString("variant: general\n")
	b.WriteString("variant_description: 通用\n")
	b.WriteString("provider: deepseek\n")
	b.WriteString("model: deepseek-chat\n")
	b.WriteString("model_tier: fast\n")
	b.WriteString("system_prompt_mode: complete\n")
	b.WriteString("context_window: 64000\n")
	b.WriteString("tools_deny: [workspace_exec, filesystem, shell, bash]\n")
	b.WriteString("ownership_kind: ecosystem_preset\n")
	b.WriteString("source: imported\n")
	b.WriteString("files:\n")
	for _, pf := range rec.PromptFiles {
		b.WriteString(fmt.Sprintf("  - name: %s\n", pf.Name))
	}
	return b.String()
}

// yamlQuote wraps a string in YAML quoting if needed.
func yamlQuote(s string) string {
	// If string contains special chars (colon, hash, etc.), quote it
	if strings.ContainsAny(s, ":#{}[]&*!|>'\"@`") {
		return fmt.Sprintf("%q", s)
	}
	// If string starts with special YAML chars
	if len(s) > 0 && (s[0] == '-' || s[0] == '?' || s[0] == ' ') {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// -----------------------------------------------------------------------------
// Main
// -----------------------------------------------------------------------------

func main() {
	srcDir := flag.String("src", `F:\agency-agents-main`, "Source agency-agents directory")
	dstDir := flag.String("dst", `internal/scenario/packs/agency-pack`, "Output Pack directory")
	zhPath := flag.String("zh", `F:\agency-agents-main\scripts\i18n\agent-names-zh.json`, "Chinese translations JSON path")
	flag.Parse()

	absSrc, err := filepath.Abs(*srcDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve src path: %v\n", err)
		os.Exit(1)
	}
	absDst, err := filepath.Abs(*dstDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve dst path: %v\n", err)
		os.Exit(1)
	}

	// Load Chinese translations
	zhMap := loadZhTranslations(*zhPath)

	// Walk division directories
	entries, err := os.ReadDir(absSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read src dir: %v\n", err)
		os.Exit(1)
	}

	var records []agentRecord
	var skipped []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if nonDivisionDirs[entry.Name()] {
			continue
		}
		division := entry.Name()
		divPath := filepath.Join(absSrc, division)

		mdFiles, err := filepath.Glob(filepath.Join(divPath, "*.md"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "glob %s: %v\n", divPath, err)
			continue
		}

		for _, mdFile := range mdFiles {
			rec, err := parseAgentFile(mdFile, division)
			if err != nil {
				skipped = append(skipped, fmt.Sprintf("%s: %v", mdFile, err))
				continue
			}

			// Apply Chinese translation if available
			if zh, ok := zhMap[rec.Frontmatter.Name]; ok {
				rec.ZhName = zh.Name
				rec.ZhDescription = zh.Description
			}

			// Generate prompt files
			generatePromptFiles(&rec)

			records = append(records, rec)
		}
	}

	// Check for position key collisions
	keyCollisions := checkPositionKeyCollisions(records)
	if len(keyCollisions) > 0 {
		fmt.Fprintf(os.Stderr, "\n⚠ Position key collisions detected:\n")
		for k, files := range keyCollisions {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", k, files)
		}
		os.Exit(1)
	}

	// Create output directory
	if err := os.MkdirAll(absDst, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir dst: %v\n", err)
		os.Exit(1)
	}
	agentsDir := filepath.Join(absDst, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir agents dir: %v\n", err)
		os.Exit(1)
	}

	// Write manifest.yaml
	manifestPath := filepath.Join(absDst, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(generateManifestYAML()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Wrote %s\n", manifestPath)

	// Write taxonomy.yaml
	taxonomyPath := filepath.Join(absDst, "taxonomy.yaml")
	if err := os.WriteFile(taxonomyPath, []byte(generateTaxonomyYAML(records)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write taxonomy: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Wrote %s\n", taxonomyPath)

	// Write agent yamls + prompt files
	translated := 0
	for i := range records {
		rec := &records[i]

		// Agent yaml
		yamlPath := filepath.Join(agentsDir, rec.AgentKey+".yaml")
		if err := os.WriteFile(yamlPath, []byte(generateAgentYAML(rec)), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write agent yaml %s: %v\n", rec.AgentKey, err)
			continue
		}

		// Prompt files
		if len(rec.PromptFiles) > 0 {
			promptDir := filepath.Join(agentsDir, rec.AgentKey)
			if err := os.MkdirAll(promptDir, 0755); err != nil {
				fmt.Fprintf(os.Stderr, "mkdir prompt dir %s: %v\n", rec.AgentKey, err)
				continue
			}
			for _, pf := range rec.PromptFiles {
				pfPath := filepath.Join(promptDir, pf.Name)
				if err := os.WriteFile(pfPath, []byte(pf.Body), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "write prompt file %s/%s: %v\n", rec.AgentKey, pf.Name, err)
				}
			}
		}

		if rec.ZhName != "" {
			translated++
		}
	}

	// Summary
	fmt.Printf("\n=== Conversion Summary ===\n")
	fmt.Printf("Total agents: %d\n", len(records))
	fmt.Printf("Chinese translations applied: %d / %d\n", translated, len(records))
	fmt.Printf("Total prompt files: %d\n", countPromptFiles(records))
	fmt.Printf("Companies: %d\n", countCompanies(records))
	fmt.Printf("Departments: %d\n", countDepartments(records))
	if len(skipped) > 0 {
		fmt.Printf("\nSkipped files (%d):\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("  %s\n", s)
		}
	}

	// Write translation manifest for agents without Chinese names
	writeTranslationManifest(records, absDst)
}

// parseAgentFile reads and parses a single agency-agents .md file.
func parseAgentFile(path, division string) (agentRecord, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return agentRecord{}, fmt.Errorf("read file: %w", err)
	}

	fm, body, err := parseFrontmatter(string(content))
	if err != nil {
		return agentRecord{}, err
	}

	intro, sections := splitSections(body)

	basename := strings.TrimSuffix(filepath.Base(path), ".md")
	dept, posKey := classifyAgent(division, basename)

	return agentRecord{
		FileBasename: basename,
		Division:     division,
		Frontmatter:  fm,
		Sections:      sections,
		IntroPara:    intro,
		Dept:         dept,
		PositionKey:  posKey,
		AgentKey:     posKey + "__general",
	}, nil
}

// loadZhTranslations loads the Chinese translations JSON file.
func loadZhTranslations(path string) map[string]zhTranslation {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read zh translations: %v\n", err)
		return map[string]zhTranslation{}
	}
	var m map[string]zhTranslation
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not parse zh translations: %v\n", err)
		return map[string]zhTranslation{}
	}
	return m
}

// checkPositionKeyCollisions finds agents with the same position key.
func checkPositionKeyCollisions(records []agentRecord) map[string][]string {
	seen := make(map[string][]string)
	for _, rec := range records {
		seen[rec.PositionKey] = append(seen[rec.PositionKey], rec.FileBasename)
	}
	collisions := make(map[string][]string)
	for k, files := range seen {
		if len(files) > 1 {
			collisions[k] = files
		}
	}
	return collisions
}

// countPromptFiles returns total prompt files across all agents.
func countPromptFiles(records []agentRecord) int {
	total := 0
	for _, rec := range records {
		total += len(rec.PromptFiles)
	}
	return total
}

// countCompanies returns the number of unique companies.
func countCompanies(records []agentRecord) int {
	seen := make(map[string]bool)
	for _, rec := range records {
		seen[rec.Dept.CompanyKey] = true
	}
	return len(seen)
}

// countDepartments returns the number of unique departments.
func countDepartments(records []agentRecord) int {
	seen := make(map[string]bool)
	for _, rec := range records {
		seen[rec.Dept.CompanyKey + "/" + rec.Dept.DeptKey] = true
	}
	return len(seen)
}

// writeTranslationManifest writes a JSON file listing agents that still need
// Chinese translation (name + description).
func writeTranslationManifest(records []agentRecord, dstDir string) {
	type entry struct {
		EnglishName        string `json:"english_name"`
		EnglishDescription string `json:"english_description"`
		Division           string `json:"division"`
		PositionKey        string `json:"position_key"`
	}

	var needTranslation []entry
	for _, rec := range records {
		if rec.ZhName == "" {
			needTranslation = append(needTranslation, entry{
				EnglishName:        rec.Frontmatter.Name,
				EnglishDescription: rec.Frontmatter.Description,
				Division:           rec.Division,
				PositionKey:        rec.PositionKey,
			})
		}
	}

	if len(needTranslation) == 0 {
		return
	}

	manifestPath := filepath.Join(dstDir, "_translation_manifest.json")
	data, _ := json.MarshalIndent(needTranslation, "", "  ")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write translation manifest: %v\n", err)
		return
	}
	fmt.Printf("\n✓ Wrote translation manifest: %s (%d agents need translation)\n",
		manifestPath, len(needTranslation))
}
