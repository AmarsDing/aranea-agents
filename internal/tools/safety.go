package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"aranea-agents/internal/tools/alias"
)

// ToolSafety classifies a tool's concurrency safety.
// ConcurrentSafe tools are read-only and safe to parallelize and cache.
// Exclusive tools modify state and must be serialized; they are never cached.
type ToolSafety int

const (
	// SafetyConcurrentSafe indicates a read-only tool safe for parallel
	// execution and deterministic result caching.
	SafetyConcurrentSafe ToolSafety = iota
	// SafetyExclusive indicates a state-modifying tool that must be
	// serialized and never cached.
	SafetyExclusive
)

// runtimeNameToRegistry maps mounted declaration names and catalog keys to
// Registry() names so ClassifyTool works for exec_command, not just hostexec.
var runtimeNameToRegistry = map[string]string{
	"read_file":           "file",
	"read_multiple_files": "file",
	"save_file":           "file",
	"list_file":           "file",
	"search_file":         "file",
	"search_content":      "file",
	"replace_content":     "file",
	"diff_edit":           "file",
	"patch_file":          "file",
	"read_lints":          "read_lints",
	"exec_command":        "hostexec",
	"write_stdin":         "hostexec",
	"kill_session":        "hostexec",
	"shell_exec":          "hostexec",
	"send_email":          "email",
	"todo_write":          "todo",
	"web_fetch":           "httpfetch",
	"gemini_web_fetch":    "geminifetch",
	"duckduckgo_search":   "duckduckgo",
	"wikipedia_search":    "wikipedia",
}

var exclusiveMutexes sync.Map // key -> *sync.RWMutex

// ClassifyTool returns the safety classification for a tool by name.
// The classification is derived from the registry's SupportsConcurrency
// field after resolving runtime aliases and ToolSet child names. Unknown
// tools default to SafetyExclusive (safe default).
func ClassifyTool(name string) ToolSafety {
	canonical := canonicalRuntimeName(name)
	// File writes inherit the parent "file" registry's SupportsConcurrency,
	// but mutating the workspace is Exclusive: they must not be cached or
	// retried, and they take the per-path write lock.
	if IsolationStrategyForTool(canonical) == IsolationStrategyWorktree {
		return SafetyExclusive
	}
	regName := registryNameFor(canonical)
	if regName == "" {
		return SafetyExclusive
	}
	for _, reg := range Registry() {
		if strings.EqualFold(reg.Name, regName) {
			if reg.SupportsConcurrency {
				return SafetyConcurrentSafe
			}
			return SafetyExclusive
		}
	}
	return SafetyExclusive
}

// IsCacheable reports whether a tool result may be reused for identical
// arguments for the lifetime of the decorator instance (the LLM agent
// snapshot, which is not refreshed per turn). Workspace file tools are
// never cacheable: a later save_file in the same agent would otherwise
// make read_file return stale content. Network ConcurrentSafe tools
// (web_fetch / search) remain cacheable.
func IsCacheable(name string) bool {
	if ClassifyTool(name) != SafetyConcurrentSafe {
		return false
	}
	reg := registryNameFor(name)
	if reg == "file" || reg == "read_lints" {
		return false
	}
	return true
}

// CatalogResultCacheAllowed reports whether Before/AfterTool ResultCache
// may store this tool. The decorator already caches IsCacheable network
// tools (invocation-scoped TTL 60s); catalog cache_enabled must not
// duplicate that. Writes and workspace files are never catalog-cached.
func CatalogResultCacheAllowed(name string) bool {
	if IsCacheable(name) {
		return false
	}
	if ClassifyTool(name) != SafetyConcurrentSafe {
		return false
	}
	reg := registryNameFor(name)
	if reg == "file" || reg == "read_lints" {
		return false
	}
	return true
}

// ExclusiveMutexKey returns the process-wide mutex family for a tool, or
// empty when the call may run concurrently. File-write tools share the
// "file_write" family (Call further narrows to per-target-path keys);
// hostexec session tools share "hostexec"; other Exclusive tools serialize
// per registry/runtime name.
func ExclusiveMutexKey(name string) string {
	return exclusiveLockKey(name, nil)
}

func exclusiveLockKey(name string, jsonArgs []byte) string {
	canonical := canonicalRuntimeName(name)
	if IsolationStrategyForTool(canonical) == IsolationStrategyWorktree {
		if path := fileWriteTargetPath(jsonArgs); path != "" {
			return "file_write:" + path
		}
		return "file_write"
	}
	if ClassifyTool(canonical) != SafetyExclusive {
		return ""
	}
	if reg := registryNameFor(canonical); reg != "" {
		return strings.ToLower(reg)
	}
	return strings.ToLower(canonical)
}

// fileWriteTargetPath extracts the workspace-relative path from file-tool
// arguments so parallel writes to different files need not share one lock.
func fileWriteTargetPath(jsonArgs []byte) string {
	if len(jsonArgs) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(jsonArgs, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"file_name", "file_path", "filepath", "path"} {
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
	return ""
}

// normalizeFileLockPath makes lock identity case-insensitive so Windows
// (and case-preserving-but-insensitive volumes) serialize writes to the
// same file. Distinct-case names on Linux may share a lock; that only
// extra-serializes, it does not allow a torn write.
func normalizeFileLockPath(p string) string {
	return strings.ToLower(filepath.ToSlash(filepath.Clean(p)))
}

func lockExclusiveTool(name string, jsonArgs []byte) func() {
	if reqs := fileLockRequests(name, jsonArgs); len(reqs) > 0 {
		return filePathLocks.acquire(reqs)
	}
	key := exclusiveLockKey(name, jsonArgs)
	if key == "" {
		return func() {}
	}
	mu := exclusiveMutexFor(key)
	mu.Lock()
	return mu.Unlock
}

func exclusiveMutexFor(key string) *sync.RWMutex {
	actual, _ := exclusiveMutexes.LoadOrStore(key, &sync.RWMutex{})
	return actual.(*sync.RWMutex)
}

func registryNameFor(name string) string {
	name = canonicalRuntimeName(name)
	if name == "" {
		return ""
	}
	for _, reg := range Registry() {
		if strings.EqualFold(reg.Name, name) {
			return reg.Name
		}
	}
	if parent, ok := runtimeNameToRegistry[strings.ToLower(name)]; ok {
		return parent
	}
	return ""
}

func canonicalRuntimeName(name string) string {
	name = strings.TrimSpace(name)
	seen := make(map[string]struct{}, 4)
	for name != "" {
		if _, dup := seen[name]; dup {
			break
		}
		seen[name] = struct{}{}
		canonical, ok := alias.RuntimeToolNameAliases[name]
		if !ok {
			break
		}
		name = canonical
	}
	return name
}
