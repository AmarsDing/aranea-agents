package filenorm

import (
	"encoding/json"
	"testing"
)

func TestNormalizeFileArgs_PathToFileName(t *testing.T) {
	out := NormalizeFileArgs("read_file", []byte(`{"path":"src/a.go"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "src/a.go" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
	if _, ok := m["path"]; ok {
		t.Fatal("path alias should be removed after mapping")
	}
}

func TestNormalizeFileArgs_PreservesFileName(t *testing.T) {
	in := []byte(`{"file_name":"a.go","path":"ignored.go"}`)
	out := NormalizeFileArgs("read_file", in)
	if string(out) != string(in) {
		t.Fatalf("expected unchanged when file_name present, got %s", out)
	}
}

func TestNormalizeFileArgs_SaveFileContentAlias(t *testing.T) {
	out := NormalizeFileArgs("save_file", []byte(`{"path":"a.go","content":"hello"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "a.go" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
	if m["contents"] != "hello" {
		t.Fatalf("contents = %v", m["contents"])
	}
}

func TestNormalizeFileArgs_ListFileDirAlias(t *testing.T) {
	out := NormalizeFileArgs("list_file", []byte(`{"dir":"src"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["path"] != "src" {
		t.Fatalf("path = %v", m["path"])
	}
}

func TestNormalizeFileArgs_ReplaceContentAliases(t *testing.T) {
	out := NormalizeFileArgs("replace_content", []byte(`{"path":"a.go","old":"foo","new":"bar"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "a.go" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
	if m["old_string"] != "foo" {
		t.Fatalf("old_string = %v", m["old_string"])
	}
	if m["new_string"] != "bar" {
		t.Fatalf("new_string = %v", m["new_string"])
	}
}

func TestNormalizeFileArgs_SearchFileGlobAlias(t *testing.T) {
	out := NormalizeFileArgs("search_file", []byte(`{"dir":"src","glob":"*.go"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["path"] != "src" {
		t.Fatalf("path = %v", m["path"])
	}
	if m["pattern"] != "*.go" {
		t.Fatalf("pattern = %v", m["pattern"])
	}
}

func TestNormalizeFileArgs_UnknownToolPassthrough(t *testing.T) {
	in := []byte(`{"path":"a.go"}`)
	if string(NormalizeFileArgs("web_fetch", in)) != string(in) {
		t.Fatal("unknown tools must not be rewritten")
	}
}

func TestNormalizeFileArgs_InvalidJSON(t *testing.T) {
	in := []byte(`not json`)
	if string(NormalizeFileArgs("read_file", in)) != string(in) {
		t.Fatal("invalid JSON must pass through")
	}
}
