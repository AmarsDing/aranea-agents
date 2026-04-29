// L4 轻量实体抽取：基于种子词典的匹配（原 internal/service/memory_l4_extractor.go）。
package application

import (
	mem "arenea/backend/internal/memory/domain"

	"strings"
	"unicode"
)

// extractionTerm 为种子词典的一行。别名在源文本中按词边界、不区分大小写匹配。
type extractionTerm struct {
	Name    string
	Type    mem.EntityType
	Aliases []string
}

// extractionDictionary 为人工筛选的种子表。
var extractionDictionary = []extractionTerm{
	{Name: "React", Type: mem.EntityFramework, Aliases: []string{"react", "reactjs", "react.js"}},
	{Name: "React 19", Type: mem.EntityFramework, Aliases: []string{"react 19", "react19"}},
	{Name: "Vue", Type: mem.EntityFramework, Aliases: []string{"vue", "vuejs", "vue.js"}},
	{Name: "Vue 3", Type: mem.EntityFramework, Aliases: []string{"vue 3", "vue3"}},
	{Name: "Svelte", Type: mem.EntityFramework, Aliases: []string{"svelte", "sveltekit"}},
	{Name: "Next.js", Type: mem.EntityFramework, Aliases: []string{"nextjs", "next.js"}},
	{Name: "Nuxt", Type: mem.EntityFramework, Aliases: []string{"nuxt", "nuxtjs", "nuxt.js"}},
	{Name: "Vite", Type: mem.EntityFramework, Aliases: []string{"vite"}},
	{Name: "Vitest", Type: mem.EntityFramework, Aliases: []string{"vitest"}},
	{Name: "Jest", Type: mem.EntityFramework, Aliases: []string{"jest"}},
	{Name: "Tailwind", Type: mem.EntityFramework, Aliases: []string{"tailwind", "tailwindcss"}},
	{Name: "Quasar", Type: mem.EntityFramework, Aliases: []string{"quasar"}},
	{Name: "FastAPI", Type: mem.EntityFramework, Aliases: []string{"fastapi"}},
	{Name: "Django", Type: mem.EntityFramework, Aliases: []string{"django"}},
	{Name: "Flask", Type: mem.EntityFramework, Aliases: []string{"flask"}},
	{Name: "Spring Boot", Type: mem.EntityFramework, Aliases: []string{"spring boot", "springboot"}},

	{Name: "Go", Type: mem.EntityTech, Aliases: []string{"golang"}},
	{Name: "TypeScript", Type: mem.EntityTech, Aliases: []string{"typescript"}},
	{Name: "JavaScript", Type: mem.EntityTech, Aliases: []string{"javascript"}},
	{Name: "Python", Type: mem.EntityTech, Aliases: []string{"python"}},
	{Name: "Rust", Type: mem.EntityTech, Aliases: []string{"rust"}},
	{Name: "Java", Type: mem.EntityTech, Aliases: []string{"java"}},
	{Name: "Kotlin", Type: mem.EntityTech, Aliases: []string{"kotlin"}},
	{Name: "Swift", Type: mem.EntityTech, Aliases: []string{"swift"}},
	{Name: "C#", Type: mem.EntityTech, Aliases: []string{"c#", "csharp"}},
	{Name: "C++", Type: mem.EntityTech, Aliases: []string{"c++", "cpp"}},

	{Name: "Postgres", Type: mem.EntityTech, Aliases: []string{"postgres", "postgresql"}},
	{Name: "MySQL", Type: mem.EntityTech, Aliases: []string{"mysql"}},
	{Name: "SQLite", Type: mem.EntityTech, Aliases: []string{"sqlite", "sqlite3"}},
	{Name: "Redis", Type: mem.EntityTech, Aliases: []string{"redis"}},
	{Name: "MongoDB", Type: mem.EntityTech, Aliases: []string{"mongodb", "mongo"}},
	{Name: "ClickHouse", Type: mem.EntityTech, Aliases: []string{"clickhouse"}},
	{Name: "Kafka", Type: mem.EntityTech, Aliases: []string{"kafka"}},
	{Name: "RabbitMQ", Type: mem.EntityTech, Aliases: []string{"rabbitmq"}},
	{Name: "Elasticsearch", Type: mem.EntityTech, Aliases: []string{"elasticsearch", "elastic search"}},

	{Name: "Docker", Type: mem.EntityTech, Aliases: []string{"docker"}},
	{Name: "Kubernetes", Type: mem.EntityTech, Aliases: []string{"kubernetes", "k8s"}},
	{Name: "AWS", Type: mem.EntityCompany, Aliases: []string{"aws", "amazon web services"}},
	{Name: "GCP", Type: mem.EntityCompany, Aliases: []string{"gcp", "google cloud"}},
	{Name: "Azure", Type: mem.EntityCompany, Aliases: []string{"azure"}},

	{Name: "OpenAI", Type: mem.EntityCompany, Aliases: []string{"openai"}},
	{Name: "Anthropic", Type: mem.EntityCompany, Aliases: []string{"anthropic"}},
	{Name: "Google", Type: mem.EntityCompany, Aliases: []string{"google"}},
	{Name: "Microsoft", Type: mem.EntityCompany, Aliases: []string{"microsoft"}},

	{Name: "GPT-4", Type: mem.EntityTech, Aliases: []string{"gpt-4", "gpt4"}},
	{Name: "Claude", Type: mem.EntityTech, Aliases: []string{"claude"}},
	{Name: "Gemini", Type: mem.EntityTech, Aliases: []string{"gemini"}},
}

// extractionMatch 为扫描器产出的一则（规范名、实体类型）命中，别名列表仅保留
// 源文中实际出现的变体。
type extractionMatch struct {
	Name    string
	Type    mem.EntityType
	Aliases []string
}

// scanExtractionMatches 对 `text` 遍历词典，每个规范名至多一条。匹配顺序稳定，
// 重复调用报告一致。
func scanExtractionMatches(text string) []extractionMatch {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lower := " " + strings.ToLower(text) + " "
	seen := map[string]int{}
	var out []extractionMatch
	for _, term := range extractionDictionary {
		var observed []string
		for _, alias := range term.Aliases {
			if alias == "" {
				continue
			}
			if !containsWord(lower, alias) {
				continue
			}
			observed = append(observed, alias)
		}
		if len(observed) == 0 {
			continue
		}
		key := string(term.Type) + "|" + strings.ToLower(term.Name)
		if idx, ok := seen[key]; ok {
			out[idx].Aliases = mergeExtractionAliases(out[idx].Aliases, observed)
			continue
		}
		seen[key] = len(out)
		out = append(out, extractionMatch{
			Name:    term.Name,
			Type:    term.Type,
			Aliases: observed,
		})
	}
	return out
}

func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return false
		}
		pos := start + idx
		if pos > 0 {
			r := rune(haystack[pos-1])
			if isExtractionWordChar(r) {
				start = pos + 1
				continue
			}
		}
		end := pos + len(needle)
		if end < len(haystack) {
			r := rune(haystack[end])
			if isExtractionWordChar(r) {
				start = pos + 1
				continue
			}
		}
		return true
	}
}

func isExtractionWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func mergeExtractionAliases(existing, more []string) []string {
	seen := map[string]bool{}
	for _, a := range existing {
		seen[a] = true
	}
	out := append([]string(nil), existing...)
	for _, a := range more {
		if seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}
