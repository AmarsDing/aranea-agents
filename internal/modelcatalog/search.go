package modelcatalog

import (
	"encoding/json"
	"strings"
)

// MaxCatalogSearchBlocks caps in-memory matches for model-wide queries (e.g. "gpt").
const MaxCatalogSearchBlocks = 200

// SearchCatalogBlocks returns pretty-printed JSON blocks (provider or provider+model).
// Empty q browses providers; non-empty q matches provider id/name or model id/name.
// truncated is true when matches were capped at MaxCatalogSearchBlocks.
func SearchCatalogBlocks(cat Catalog, q string, limit, offset int) (blocks []string, total int, truncated bool) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	q = strings.ToLower(strings.TrimSpace(q))

	all := make([]string, 0, 32)
	providers := ListProviders(cat, "", len(cat)+1, 0)

	appendBlock := func(body string) bool {
		if len(all) >= MaxCatalogSearchBlocks {
			truncated = true
			return false
		}
		all = append(all, body)
		return true
	}

	if q == "" {
		for _, p := range providers {
			b, err := json.MarshalIndent(p, "", "  ")
			if err != nil {
				continue
			}
			if !appendBlock(string(b)) {
				break
			}
		}
	} else {
		for _, p := range providers {
			if truncated {
				break
			}
			pHay := strings.ToLower(p.ID + " " + p.Name)
			if strings.Contains(pHay, q) {
				b, err := json.MarshalIndent(p, "", "  ")
				if err != nil {
					continue
				}
				if !appendBlock(string(b)) {
					break
				}
				continue
			}
			for _, m := range ListModels(p, q, true, len(p.Models)+1, 0) {
				wrapper := map[string]any{
					"provider": map[string]any{
						"id":   p.ID,
						"name": p.Name,
						"api":  p.API,
						"doc":  p.Doc,
					},
					"model": m,
				}
				b, err := json.MarshalIndent(wrapper, "", "  ")
				if err != nil {
					continue
				}
				if !appendBlock(string(b)) {
					break
				}
			}
		}
	}

	total = len(all)
	if offset >= total {
		return []string{}, total, truncated
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total, truncated
}

// IsCatalogJSONBlock reports whether s looks like a complete JSON object block.
func IsCatalogJSONBlock(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return false
	}
	var v any
	return json.Unmarshal([]byte(s), &v) == nil
}
