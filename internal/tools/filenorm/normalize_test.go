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

func TestNormalizeFileArgs_SearchContentAliases(t *testing.T) {
	out := NormalizeFileArgs("search_content", []byte(`{"dir":"src","glob":"*.go","query":"TODO"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["path"] != "src" {
		t.Fatalf("path = %v", m["path"])
	}
	if m["file_pattern"] != "*.go" {
		t.Fatalf("file_pattern = %v", m["file_pattern"])
	}
	if m["content_pattern"] != "TODO" {
		t.Fatalf("content_pattern = %v", m["content_pattern"])
	}
}

func TestNormalizeFileArgs_SearchContentPatternAsContent(t *testing.T) {
	out := NormalizeFileArgs("search_content", []byte(`{"pattern":"func main"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["content_pattern"] != "func main" {
		t.Fatalf("content_pattern = %v", m["content_pattern"])
	}
}

func TestNormalizeFileArgs_ReadFileLineAliases(t *testing.T) {
	out := NormalizeFileArgs("read_file", []byte(`{"path":"a.go","start":10,"limit":20}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "a.go" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
	if m["start_line"] != float64(10) {
		t.Fatalf("start_line = %v", m["start_line"])
	}
	if m["num_lines"] != float64(20) {
		t.Fatalf("num_lines = %v", m["num_lines"])
	}
}

func TestNormalizeFileArgs_SearchContentGrepDetails(t *testing.T) {
	out := NormalizeFileArgs("search_content", []byte(`{"query":"TODO","-A":2,"-B":1,"file_type":"go","-U":true,"limit":5,"skip":1}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["content_pattern"] != "TODO" {
		t.Fatalf("content_pattern = %v", m["content_pattern"])
	}
	if m["after"] != float64(2) {
		t.Fatalf("after = %v", m["after"])
	}
	if m["before"] != float64(1) {
		t.Fatalf("before = %v", m["before"])
	}
	if m["type"] != "go" {
		t.Fatalf("type = %v", m["type"])
	}
	if m["multiline"] != true {
		t.Fatalf("multiline = %v", m["multiline"])
	}
	if m["head_limit"] != float64(5) {
		t.Fatalf("head_limit = %v", m["head_limit"])
	}
	if m["offset"] != float64(1) {
		t.Fatalf("offset = %v", m["offset"])
	}
}

func TestNormalizeFileArgs_DeleteFilePathAlias(t *testing.T) {
	out := NormalizeFileArgs("delete_file", []byte(`{"path":"tmp/notes.txt"}`))
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["file_name"] != "tmp/notes.txt" {
		t.Fatalf("file_name = %v", m["file_name"])
	}
}

func TestNormalizeFileArgs_InvalidJSON(t *testing.T) {
	in := []byte(`not json`)
	if string(NormalizeFileArgs("read_file", in)) != string(in) {
		t.Fatal("invalid JSON must pass through")
	}
}
