package modelregistry

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
)

type RuntimeProfile struct {
	ProviderType string `json:"provider_type"`
	Variant      string `json:"variant,omitempty"`
	AuthType     string `json:"auth_type"`
	APIBaseURL   string `json:"api_base_url,omitempty"`
}

//go:embed runtime_overlay.json
var runtimeOverlayJSON []byte

var runtimeOverlay map[string]RuntimeProfile

func init() {
	runtimeOverlay = map[string]RuntimeProfile{}
	_ = json.Unmarshal(runtimeOverlayJSON, &runtimeOverlay)
}

func RuntimeProfileFor(providerID string) (RuntimeProfile, bool) {
	p, ok := runtimeOverlay[strings.TrimSpace(providerID)]
	return p, ok
}

var ProviderMigration = map[string]string{
	"aliyun-qwen":     "alibaba-cn",
	"tencent-hunyuan": "hunyuan",
	"moonshot-kimi":   "moonshotai-cn",
	"zhipu-glm":       "zhipuai",
	"gemini":          "google",
}

func CountProviders(cat Directory, q string) int {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return len(cat)
	}
	n := 0
	for _, p := range cat {
		hay := strings.ToLower(p.ID + " " + p.Name)
		if strings.Contains(hay, q) {
			n++
		}
	}
	return n
}

func ListProviders(cat Directory, q string, limit, offset int) []Provider {
	if limit <= 0 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	q = strings.ToLower(strings.TrimSpace(q))
	matched := make([]Provider, 0, len(cat))
	for _, p := range cat {
		if q != "" {
			hay := strings.ToLower(p.ID + " " + p.Name)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		matched = append(matched, p)
	}
	sort.Slice(matched, func(i, j int) bool {
		ni, nj := strings.ToLower(matched[i].Name), strings.ToLower(matched[j].Name)
		if ni != nj {
			return ni < nj
		}
		return matched[i].ID < matched[j].ID
	})
	if offset >= len(matched) {
		return nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end]
}

func CountModels(p Provider, q string, includeDeprecated bool) int {
	q = strings.ToLower(strings.TrimSpace(q))
	n := 0
	for _, m := range p.Models {
		if !includeDeprecated && strings.EqualFold(m.Status, "deprecated") {
			continue
		}
		if q != "" {
			hay := strings.ToLower(m.ID + " " + m.Name)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		n++
	}
	return n
}

func ListModels(p Provider, q string, includeDeprecated bool, limit, offset int) []Model {
	if limit <= 0 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	q = strings.ToLower(strings.TrimSpace(q))
	matched := make([]Model, 0, len(p.Models))
	for _, m := range p.Models {
		if !includeDeprecated && strings.EqualFold(m.Status, "deprecated") {
			continue
		}
		if q != "" {
			hay := strings.ToLower(m.ID + " " + m.Name)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		matched = append(matched, m)
	}
	sort.Slice(matched, func(i, j int) bool {
		si, sj := strings.ToLower(matched[i].Status), strings.ToLower(matched[j].Status)
		if si == "deprecated" && sj != "deprecated" {
			return false
		}
		if si != "deprecated" && sj == "deprecated" {
			return true
		}
		ni, nj := strings.ToLower(matched[i].Name), strings.ToLower(matched[j].Name)
		if ni != nj {
			return ni < nj
		}
		return matched[i].ID < matched[j].ID
	})
	if offset >= len(matched) {
		return nil
	}
	end := offset + limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[offset:end]
}
