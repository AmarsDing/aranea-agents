package tools

import (
	"testing"
)

func TestFileLockRequests_WriteAndList(t *testing.T) {
	write := fileLockRequests("save_file", []byte(`{"file_name":"src/a.go"}`))
	if len(write) != 1 || !write[0].exclusive || write[0].path != "src/a.go" {
		t.Fatalf("write req = %+v", write)
	}
	list := fileLockRequests("list_file", []byte(`{"path":"src"}`))
	if len(list) != 1 || list[0].exclusive || !list[0].cover || list[0].path != "src" {
		t.Fatalf("list req = %+v", list)
	}
	if !fileLockConflicts(write[0], list[0]) {
		t.Fatal("list src should conflict with write src/a.go")
	}
	other := fileLockRequests("list_file", []byte(`{"path":"pkg"}`))
	if fileLockConflicts(write[0], other[0]) {
		t.Fatal("list pkg should not conflict with write src/a.go")
	}
}

func TestFileLockRequests_SearchCoversRoot(t *testing.T) {
	reqs := fileLockRequests("search_content", []byte(`{}`))
	if len(reqs) != 1 || reqs[0].path != "." || !reqs[0].cover {
		t.Fatalf("empty search should cover workspace root, got %+v", reqs)
	}
}

func TestGlobCoverPath(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"*.go", "."},
		{"src/*.go", "src"},
		{"**/*.md", "."},
		{"pkg/foo.go", "pkg"},
	}
	for _, tt := range tests {
		if got := globCoverPath(tt.in); got != tt.want {
			t.Errorf("globCoverPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPathIsUnder(t *testing.T) {
	if !pathIsUnder("src/a.go", "src") {
		t.Fatal("src/a.go should be under src")
	}
	if pathIsUnder("src", "src") {
		t.Fatal("equal path is not under itself")
	}
	if !pathIsUnder("src/a.go", ".") {
		t.Fatal("file should be under workspace root")
	}
}
