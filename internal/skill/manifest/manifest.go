package manifest

import (
	"strings"

	"aranea-agents/internal/biz"
)

type Manifest struct {
	Name        string
	Description string
	Tags        []biz.SkillTag
	Triggers    []string
	Tools       []string
	Variables   map[string]string
	Body        string
}

func Parse(body string) Manifest {
	m := Manifest{}
	// P-r4-BOM：剔除 UTF-8 BOM（U+FEFF）。Windows 编辑器/PowerShell 写出的
	// SKILL.md 常带 BOM，否则首行 "\ufeff---" 无法匹配 frontmatter 起始，
	// 导致 name/description/tags 全部解析失败（description 退化为 "---"）。
	body = strings.TrimPrefix(body, "\ufeff")
	lines := strings.Split(body, "\n")
	inFrontmatter := false
	fmEnd := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFrontmatter = true
		for i := 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "---" {
				fmEnd = i + 1
				break
			}
			if key, value, ok := splitMetaLine(trimmed); ok {
				// YAML block scalar（| / |- / > / >- 等）：消费后续缩进行作为值。
				// alibabacloud 系列 SKILL.md 的多行 description 即此格式，
				// 不处理会把字面量 "|" / ">" 存为描述。
				if isBlockScalarIndicator(value) {
					folded := strings.HasPrefix(value, ">")
					var buf []string
					j := i + 1
					for ; j < len(lines); j++ {
						raw := lines[j]
						if strings.TrimSpace(raw) == "---" {
							break
						}
						if raw != "" && !strings.HasPrefix(raw, " ") && !strings.HasPrefix(raw, "\t") {
							break // dedent：块结束
						}
						buf = append(buf, strings.TrimSpace(raw))
					}
					for len(buf) > 0 && buf[len(buf)-1] == "" {
						buf = buf[:len(buf)-1]
					}
					if folded {
						value = strings.Join(buf, " ")
					} else {
						value = strings.Join(buf, "\n")
					}
					i = j - 1
				}
				switch strings.ToLower(key) {
				case "name", "title":
					m.Name = strings.Trim(value, `"'`)
				case "description", "summary":
					m.Description = strings.Trim(value, `"'`)
				case "tags":
					for _, tag := range strings.Split(strings.Trim(value, "[]"), ",") {
						tag = strings.Trim(strings.TrimSpace(tag), `"'`)
						if tag != "" {
							m.Tags = append(m.Tags, biz.SkillTag{Name: tag, Source: "user"})
						}
					}
				case "triggers":
					for _, t := range strings.Split(strings.Trim(value, "[]"), ",") {
						t = strings.Trim(strings.TrimSpace(t), `"'`)
						if t != "" {
							m.Triggers = append(m.Triggers, t)
						}
					}
				case "tools":
					for _, t := range strings.Split(strings.Trim(value, "[]"), ",") {
						t = strings.Trim(strings.TrimSpace(t), `"'`)
						if t != "" {
							m.Tools = append(m.Tools, t)
						}
					}
				default:
					if strings.HasPrefix(strings.ToLower(key), "var_") || strings.HasPrefix(strings.ToLower(key), "variable_") {
						if m.Variables == nil {
							m.Variables = make(map[string]string)
						}
						m.Variables[key] = strings.Trim(value, `"'`)
					}
				}
			}
		}
	}
	if fmEnd < len(lines) {
		m.Body = strings.Join(lines[fmEnd:], "\n")
	} else {
		m.Body = body
	}
	if m.Name == "" {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "# ") {
				m.Name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
				break
			}
		}
	}
	if m.Description == "" && !inFrontmatter {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				m.Description = trimmed
				break
			}
		}
	}
	return m
}

func splitMetaLine(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+1:]), true
}

// isBlockScalarIndicator 判定 YAML block scalar 指示符（| 保留换行 / > 折叠，
// 可带 - + chomping 修饰）。
func isBlockScalarIndicator(v string) bool {
	switch v {
	case "|", "|-", "|+", ">", ">-", ">+":
		return true
	}
	return false
}
