package tools

import (
	"encoding/json"
	"testing"
)

func TestNormalizeInvocation_FilePathAlias(t *testing.T) {
	out := NormalizeInvocation("read_file", []byte(`{"path":"src/a.go"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "src/a.go" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
}

func TestNormalizeInvocation_WriteFileAliasName(t *testing.T) {
	out := NormalizeInvocation("write_file", []byte(`{"path":"a.go","content":"x"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "a.go" || m["contents"] != "x" {
		t.Fatalf("got %s", out)
	}
}

func TestNormalizeInvocation_ExecArray(t *testing.T) {
	out := NormalizeInvocation("exec_command", []byte(`{"command":["ls","-la"]}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["command"] != "ls -la" {
		t.Fatalf("command = %v", m["command"])
	}
}

func TestNormalizeInvocation_WebFetchURL(t *testing.T) {
	out := NormalizeInvocation("web_fetch", []byte(`{"url":"https://example.com"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	urls, ok := m["urls"].([]any)
	if !ok || len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("urls = %v", m["urls"])
	}
}

func TestNormalizeInvocation_Idempotent(t *testing.T) {
	in := []byte(`{"path":"a.go"}`)
	once := NormalizeInvocation("read_file", in)
	twice := NormalizeInvocation("read_file", once)
	if string(once) != string(twice) {
		t.Fatalf("second pass changed %s -> %s", once, twice)
	}
}

func TestNormalizeInvocation_UnknownPassthrough(t *testing.T) {
	in := []byte(`{"q":"x"}`)
	if string(NormalizeInvocation("todo_write", in)) != string(in) {
		t.Fatal("unknown tools must pass through")
	}
}

func TestNormalizeInvocation_AliasRewriteCounted(t *testing.T) {
	before := AliasRewriteTotal()
	out := NormalizeInvocation("read_file", []byte(`{"path":"a.go"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "a.go" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
	if AliasRewriteTotal() != before+1 {
		t.Fatalf("rewrite count: got %d want %d", AliasRewriteTotal(), before+1)
	}
	// Idempotent second pass must not count again.
	_ = NormalizeInvocation("read_file", out)
	if AliasRewriteTotal() != before+1 {
		t.Fatalf("idempotent pass counted a rewrite: %d", AliasRewriteTotal())
	}
}

func TestNormalizeInvocation_CanonicalNotCounted(t *testing.T) {
	before := AliasRewriteTotal()
	in := []byte(`{"file_name":"a.go"}`)
	if string(NormalizeInvocation("read_file", in)) != string(in) {
		t.Fatal("canonical args must pass through")
	}
	if AliasRewriteTotal() != before {
		t.Fatalf("canonical args counted as rewrite: %d -> %d", before, AliasRewriteTotal())
	}
}
