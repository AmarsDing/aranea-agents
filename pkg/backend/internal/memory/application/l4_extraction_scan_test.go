package application

import "testing"

// scanExtractionMatches 须尊重词边界 — "react" 可匹配 "interaction" 不可；规范名只出现一次。
func TestExtractionScannerRespectsWordBoundaries(t *testing.T) {
	matches := scanExtractionMatches("Their interaction with the Reactor used React and Postgres.")
	names := map[string]bool{}
	for _, m := range matches {
		names[m.Name] = true
	}
	if !names["React"] {
		t.Fatalf("expected React match, got %#v", matches)
	}
	if !names["Postgres"] {
		t.Fatalf("expected Postgres match, got %#v", matches)
	}
	for _, m := range matches {
		if m.Name == "React" {
			for _, alias := range m.Aliases {
				if alias == "reactor" || alias == "interaction" {
					t.Fatalf("scanner matched a substring: %s", alias)
				}
			}
		}
	}
}
