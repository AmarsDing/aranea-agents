package tools

import (
	"encoding/json"
	"path"
	"strings"
	"sync"
)

// fileLockReq is one hierarchical lock on a workspace-relative path.
// exclusive is a write; cover means the holder also occupies descendants
// (directory listing / search). Shared+cover vs exclusive descendant is how
// list_file/search_* wait for in-flight writes without taking per-file mutexes
// (which would deadlock if lock order were parent-then-child vs child-then-parent).
type fileLockReq struct {
	path      string
	exclusive bool
	cover     bool
}

type filePathLockTable struct {
	mu      sync.Mutex
	cond    *sync.Cond
	nextID  uint64
	holders map[uint64][]fileLockReq
}

var filePathLocks = newFilePathLockTable()

func newFilePathLockTable() *filePathLockTable {
	t := &filePathLockTable{holders: make(map[uint64][]fileLockReq)}
	t.cond = sync.NewCond(&t.mu)
	return t
}

func (t *filePathLockTable) acquire(reqs []fileLockReq) func() {
	reqs = dedupeFileLockReqs(reqs)
	if len(reqs) == 0 {
		return func() {}
	}
	t.mu.Lock()
	for t.conflictsAny(reqs) {
		t.cond.Wait()
	}
	t.nextID++
	id := t.nextID
	t.holders[id] = reqs
	t.mu.Unlock()
	return func() {
		t.mu.Lock()
		delete(t.holders, id)
		t.cond.Broadcast()
		t.mu.Unlock()
	}
}

func (t *filePathLockTable) conflictsAny(want []fileLockReq) bool {
	for _, held := range t.holders {
		for _, h := range held {
			for _, w := range want {
				if fileLockConflicts(h, w) {
					return true
				}
			}
		}
	}
	return false
}

func fileLockConflicts(held, want fileLockReq) bool {
	if !fileLockPathsRelated(held.path, want.path) {
		return false
	}
	if !held.exclusive && !want.exclusive {
		return false
	}
	return true
}

func fileLockPathsRelated(a, b string) bool {
	return a == b || pathIsUnder(a, b) || pathIsUnder(b, a)
}

func pathIsUnder(child, parent string) bool {
	if parent == "." {
		return child != "."
	}
	if child == parent {
		return false
	}
	return strings.HasPrefix(child, parent+"/")
}

func dedupeFileLockReqs(reqs []fileLockReq) []fileLockReq {
	if len(reqs) <= 1 {
		return reqs
	}
	seen := make(map[string]fileLockReq, len(reqs))
	for _, r := range reqs {
		if r.path == "" {
			continue
		}
		key := r.path
		if prev, ok := seen[key]; ok {
			if r.exclusive {
				prev.exclusive = true
			}
			if r.cover {
				prev.cover = true
			}
			seen[key] = prev
			continue
		}
		seen[key] = r
	}
	out := make([]fileLockReq, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	return out
}

func fileLockRequests(name string, jsonArgs []byte) []fileLockReq {
	switch canonicalRuntimeName(name) {
	case "save_file", "diff_edit", "patch_file", "replace_content", "delete_file":
		if p := fileWriteTargetPath(jsonArgs); p != "" {
			return []fileLockReq{{path: p, exclusive: true}}
		}
		return []fileLockReq{{path: ".", exclusive: true, cover: true}}
	case "read_file":
		if p := fileWriteTargetPath(jsonArgs); p != "" {
			return []fileLockReq{{path: p}}
		}
		return nil
	case "list_file", "search_file", "search_content":
		return []fileLockReq{{path: fileDirOrGlobCoverPath(name, jsonArgs), cover: true}}
	case "read_multiple_files":
		return readMultipleLockReqs(jsonArgs)
	default:
		return nil
	}
}

func fileDirOrGlobCoverPath(name string, jsonArgs []byte) string {
	if p := fileDirTargetPath(jsonArgs); p != "." {
		return p
	}
	keys := []string{"file_pattern", "glob", "include"}
	if canonicalRuntimeName(name) == "search_file" {
		keys = append([]string{"pattern"}, keys...)
	}
	if g := firstStringArg(jsonArgs, keys...); g != "" {
		return globCoverPath(g)
	}
	return "."
}

func firstStringArg(jsonArgs []byte, keys ...string) string {
	if len(jsonArgs) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonArgs, &payload); err != nil {
		return ""
	}
	for _, key := range keys {
		s, ok := payload[key].(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		return s
	}
	return ""
}

func fileDirTargetPath(jsonArgs []byte) string {
	if len(jsonArgs) == 0 {
		return "."
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonArgs, &payload); err != nil {
		return "."
	}
	for _, key := range []string{"path", "dir", "directory", "folder", "file_name"} {
		s, ok := payload[key].(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		return normalizeFileLockPath(s)
	}
	return "."
}

func readMultipleLockReqs(jsonArgs []byte) []fileLockReq {
	if len(jsonArgs) == 0 {
		return []fileLockReq{{path: ".", cover: true}}
	}
	var payload struct {
		Patterns []string `json:"patterns"`
	}
	if err := json.Unmarshal(jsonArgs, &payload); err != nil || len(payload.Patterns) == 0 {
		return []fileLockReq{{path: ".", cover: true}}
	}
	out := make([]fileLockReq, 0, len(payload.Patterns))
	for _, pat := range payload.Patterns {
		out = append(out, fileLockReq{path: globCoverPath(pat), cover: true})
	}
	return out
}

func globCoverPath(pattern string) string {
	p := strings.TrimSpace(filepathToSlash(pattern))
	if p == "" || strings.Contains(p, "**") || strings.HasPrefix(p, "*") {
		return "."
	}
	dir := path.Dir(p)
	if dir == "." || dir == "" {
		return "."
	}
	return normalizeFileLockPath(dir)
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
