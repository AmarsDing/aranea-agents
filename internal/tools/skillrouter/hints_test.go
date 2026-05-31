package skillrouter

import (
	"strings"
	"testing"
)

func TestExtractTagHints_XlsxAutoDetect(t *testing.T) {
	h := ExtractTagHints("帮我处理xlsx文件")
	found := false
	for _, tok := range h {
		if tok == "file_type:xlsx" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file_type:xlsx in hints, got %v", h)
	}
}

func TestExtractTagHints_FileTypeColonChinese(t *testing.T) {
	h := ExtractTagHints("file_type：pdf")
	found := false
	for _, tok := range h {
		if tok == "file_type:pdf" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected file_type:pdf in hints, got %v", h)
	}
}

func TestExtractTagHints_DomainColonChinese(t *testing.T) {
	h := ExtractTagHints("domain：finance")
	found := false
	for _, tok := range h {
		if tok == "domain:finance" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected domain:finance in hints, got %v", h)
	}
}

func TestExtractTagHints_MultipleFileTypes(t *testing.T) {
	h := ExtractTagHints("file_type:csv file_type:pdf")
	got := map[string]bool{}
	for _, tok := range h {
		got[tok] = true
	}
	if !got["file_type:csv"] {
		t.Error("missing file_type:csv")
	}
	if !got["file_type:pdf"] {
		t.Error("missing file_type:pdf")
	}
}

func TestExtractTagHints_WhitespaceOnly(t *testing.T) {
	h := ExtractTagHints("   ")
	if h != nil {
		t.Fatalf("expected nil for whitespace-only input, got %v", h)
	}
}

func TestDetectIntentPaths_ZeroMaxLeaves(t *testing.T) {
	paths := DetectIntentPaths("帮我发邮件", 0)
	if paths != nil {
		t.Fatalf("expected nil for maxLeaves=0, got %v", paths)
	}
}

func TestDetectIntentPaths_NegativeMaxLeaves(t *testing.T) {
	paths := DetectIntentPaths("帮我发邮件", -1)
	if paths != nil {
		t.Fatalf("expected nil for negative maxLeaves, got %v", paths)
	}
}

func TestDetectIntentPaths_WhitespaceQuery(t *testing.T) {
	paths := DetectIntentPaths("   ", 3)
	if paths != nil {
		t.Fatalf("expected nil for whitespace query, got %v", paths)
	}
}

func TestDetectIntentPaths_LimitResults(t *testing.T) {
	q := "读xlsx做情感分析发邮件"
	paths := DetectIntentPaths(q, 1)
	if len(paths) > 1 {
		t.Fatalf("expected at most 1 result, got %d: %v", len(paths), paths)
	}
}

func TestDetectIntentPaths_PathMatchBonus(t *testing.T) {
	leaves := TaxonomyLeaves()
	if len(leaves) == 0 {
		t.Skip("no taxonomy leaves")
	}
	targetPath := leaves[0].Path
	paths := DetectIntentPaths(targetPath, 3)
	if len(paths) == 0 {
		t.Fatalf("expected at least one path when query contains exact path, got %v", paths)
	}
	if paths[0] != targetPath {
		t.Fatalf("expected first result to be %q (bonus path), got %q", targetPath, paths[0])
	}
}

func TestTaxonomyLeaves_AllPathsUnique(t *testing.T) {
	leaves := TaxonomyLeaves()
	seen := map[string]bool{}
	for _, leaf := range leaves {
		if seen[leaf.Path] {
			t.Fatalf("duplicate path %q", leaf.Path)
		}
		seen[leaf.Path] = true
	}
}

func TestTaxonomyLeaves_AllKeywordsLowercase(t *testing.T) {
	leaves := TaxonomyLeaves()
	for _, leaf := range leaves {
		for _, kw := range leaf.Keywords {
			if kw != strings.ToLower(kw) {
				t.Errorf("keyword %q in path %q is not lowercase", kw, leaf.Path)
			}
		}
	}
}
