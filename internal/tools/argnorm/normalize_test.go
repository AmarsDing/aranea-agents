package argnorm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeArgs_WebFetchURLAlias(t *testing.T) {
	out := NormalizeArgs("web_fetch", []byte(`{"url":"https://example.com"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	urls, ok := m["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("urls = %v", m["urls"])
	}
	if _, ok := m["url"]; ok {
		t.Fatal("url alias should be removed")
	}
}

func TestNormalizeArgs_WebFetchURLStringCoerce(t *testing.T) {
	out := NormalizeArgs("web_fetch", []byte(`{"urls":"https://example.com"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	urls, ok := m["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("urls = %v", m["urls"])
	}
}

func TestNormalizeArgs_SearchQueryAliases(t *testing.T) {
	out := NormalizeArgs("duckduckgo_search", []byte(`{"q":"golang tools"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["query"] != "golang tools" {
		t.Fatalf("query = %v", m["query"])
	}
}

func TestNormalizeArgs_GeminiPromptFromURL(t *testing.T) {
	out := NormalizeArgs("gemini_web_fetch", []byte(`{"url":"https://example.com"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["prompt"] != "https://example.com" {
		t.Fatalf("prompt = %v", m["prompt"])
	}
}

func TestNormalizeArgs_UnknownToolPassthrough(t *testing.T) {
	in := []byte(`{"q":"x"}`)
	if string(NormalizeArgs("save_file", in)) != string(in) {
		t.Fatal("file tools must not be rewritten by argnorm")
	}
}

func TestNormalizeArgs_PreservesQuery(t *testing.T) {
	in := []byte(`{"query":"keep","q":"ignored"}`)
	out := NormalizeArgs("web_research", in)
	if string(out) != string(in) {
		t.Fatalf("expected unchanged when query present, got %s", out)
	}
}
