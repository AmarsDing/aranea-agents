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
	lines := strings.Split(body, "\n")
	inFrontmatter := false
	fmEnd := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inFrontmatter = true
		for i, line := range lines[1:] {
			trimmed := strings.TrimSpace(line)
			if trimmed == "---" {
				fmEnd = i + 2
				break
			}
			if key, value, ok := splitMetaLine(trimmed); ok {
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
