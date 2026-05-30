package testexec

import (
	"context"
	"testing"
)

func TestClampTimeout(t *testing.T) {
	tests := []struct {
		name string
		sec  int
		want int
	}{
		{name: "zero", sec: 0, want: DefaultTimeoutSec},
		{name: "negative", sec: -5, want: DefaultTimeoutSec},
		{name: "normal", sec: 45, want: 45},
		{name: "at max", sec: MaxTimeoutSec, want: MaxTimeoutSec},
		{name: "over max", sec: 999, want: MaxTimeoutSec},
		{name: "one", sec: 1, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampTimeout(tt.sec)
			if got != tt.want {
				t.Fatalf("clampTimeout(%d)=%d want %d", tt.sec, got, tt.want)
			}
		})
	}
}

func TestNormalizeArgsJSON(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: "{}"},
		{name: "whitespace", raw: "   ", want: "{}"},
		{name: "null literal", raw: "null", want: "{}"},
		{name: "valid object", raw: `{"key":"val"}`, want: `{"key":"val"}`},
		{name: "invalid json", raw: `{bad`, want: "{}"},
		{name: "valid array", raw: `[1,2]`, want: `[1,2]`},
		{name: "valid number", raw: "42", want: "42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeArgsJSON(tt.raw)
			if got != tt.want {
				t.Fatalf("normalizeArgsJSON(%q)=%q want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "short string", s: "hi", n: 10, want: "hi"},
		{name: "exact length", s: "hello", n: 5, want: "hello"},
		{name: "truncated", s: "hello world", n: 5, want: "hello"},
		{name: "zero n", s: "abc", n: 0, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.n)
			if got != tt.want {
				t.Fatalf("truncate(%q,%d)=%q want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestPreviewValue(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want string
	}{
		{name: "nil", v: nil, want: ""},
		{name: "string", v: "hello", want: `"hello"`},
		{name: "int", v: 42, want: "42"},
		{name: "map", v: map[string]any{"k": "v"}, want: `{"k":"v"}`},
		{name: "bool", v: true, want: "true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := previewValue(tt.v)
			if got != tt.want {
				t.Fatalf("previewValue(%v)=%q want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestCatalogToolNames(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want []string
	}{
		{name: "shell_exec", key: "shell_exec", want: []string{"shell_exec", "exec_command"}},
		{name: "other key", key: "web_research", want: []string{"web_research"}},
		{name: "empty key", key: "", want: []string{""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := catalogToolNames(tt.key)
			if len(got) != len(tt.want) {
				t.Fatalf("catalogToolNames(%q)=%v want %v", tt.key, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("catalogToolNames(%q)[%d]=%q want %q", tt.key, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestMergeConfigJSON(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		deflt     string
		want      map[string]any
	}{
		{
			name:      "both empty",
			base:      "",
			deflt:     "",
			want:      map[string]any{},
		},
		{
			name:      "base only",
			base:      `{"a":"1"}`,
			deflt:     "",
			want:      map[string]any{"a": "1"},
		},
		{
			name:      "default only",
			base:      "",
			deflt:     `{"b":"2"}`,
			want:      map[string]any{"b": "2"},
		},
		{
			name:      "both present default overwrites",
			base:      `{"a":"1"}`,
			deflt:     `{"a":"2","c":"3"}`,
			want:      map[string]any{"a": "2", "c": "3"},
		},
		{
			name:      "invalid base json",
			base:      `{bad`,
			deflt:     `{"b":"2"}`,
			want:      map[string]any{"b": "2"},
		},
		{
			name:      "empty object literal",
			base:      `{}`,
			deflt:     `{"b":"2"}`,
			want:      map[string]any{"b": "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeConfigJSON(tt.base, tt.deflt)
			if len(got) != len(tt.want) {
				t.Fatalf("mergeConfigJSON()=%v want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Fatalf("mergeConfigJSON()[%q]=%v want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestOpenAPISpecFromCatalogTool(t *testing.T) {
	tests := []struct {
		name      string
		tool      CatalogTool
		wantOK    bool
		wantName  string
		wantURL   string
		wantData  string
	}{
		{
			name:   "empty metadata",
			tool:   CatalogTool{Key: "myapi", MetadataJSON: ""},
			wantOK: false,
		},
		{
			name:   "empty object metadata",
			tool:   CatalogTool{Key: "myapi", MetadataJSON: "{}"},
			wantOK: false,
		},
		{
			name:   "invalid json metadata",
			tool:   CatalogTool{Key: "myapi", MetadataJSON: "{bad"},
			wantOK: false,
		},
		{
			name:     "openapi_spec_url",
			tool:     CatalogTool{Key: "myapi", MetadataJSON: `{"openapi_spec_url":"http://spec.example.com"}`},
			wantOK:   true,
			wantName: "myapi",
			wantURL:  "http://spec.example.com",
		},
		{
			name:     "spec_url fallback",
			tool:     CatalogTool{Key: "myapi", MetadataJSON: `{"spec_url":"http://spec2.example.com"}`},
			wantOK:   true,
			wantName: "myapi",
			wantURL:  "http://spec2.example.com",
		},
		{
			name:     "openapi_spec_data",
			tool:     CatalogTool{Key: "myapi", MetadataJSON: `{"openapi_spec_data":"{\"openapi\":\"3.0\"}"}`},
			wantOK:   true,
			wantName: "myapi",
			wantData: `{"openapi":"3.0"}`,
		},
		{
			name:     "spec_data fallback",
			tool:     CatalogTool{Key: "myapi", MetadataJSON: `{"spec_data":"{\"openapi\":\"3.1\"}"}`},
			wantOK:   true,
			wantName: "myapi",
			wantData: `{"openapi":"3.1"}`,
		},
		{
			name:   "no url and no data",
			tool:   CatalogTool{Key: "myapi", MetadataJSON: `{"other":"value"}`},
			wantOK: false,
		},
		{
			name:     "url preferred over spec_url",
			tool:     CatalogTool{Key: "myapi", MetadataJSON: `{"openapi_spec_url":"http://primary","spec_url":"http://fallback"}`},
			wantOK:   true,
			wantName: "myapi",
			wantURL:  "http://primary",
		},
		{
			name:     "whitespace key trimmed",
			tool:     CatalogTool{Key: "  myapi  ", MetadataJSON: `{"openapi_spec_url":"http://spec.example.com"}`},
			wantOK:   true,
			wantName: "myapi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := openAPISpecFromCatalogTool(tt.tool)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v wantOK=%v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if spec.Name != tt.wantName {
				t.Fatalf("Name=%q want %q", spec.Name, tt.wantName)
			}
			if tt.wantURL != "" && spec.SpecURL != tt.wantURL {
				t.Fatalf("SpecURL=%q want %q", spec.SpecURL, tt.wantURL)
			}
			if tt.wantData != "" && string(spec.SpecData) != tt.wantData {
				t.Fatalf("SpecData=%q want %q", string(spec.SpecData), tt.wantData)
			}
		})
	}
}

func TestAssemblyForCatalogKey_moreCases(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		merged     map[string]any
		wantOK     bool
		wantTool   string
	}{
		{name: "empty key", key: "", merged: nil, wantOK: false},
		{name: "whitespace key", key: "  ", merged: nil, wantOK: false},
		{name: "knowledge_search", key: "knowledge_search", merged: nil, wantOK: false},
		{name: "knowledge_reflect", key: "knowledge_reflect", merged: nil, wantOK: false},
		{name: "call_agent", key: "call_agent", merged: nil, wantOK: false},
		{name: "mcp_tool_set", key: "mcp_tool_set", merged: nil, wantOK: false},
		{name: "mcp_broker", key: "mcp_broker", merged: nil, wantOK: false},
		{name: "workspace_exec", key: "workspace_exec", merged: nil, wantOK: false},
		{name: "unknown key", key: "nonexistent_tool", merged: nil, wantOK: false},
		{name: "web_fetch", key: "web_fetch", merged: nil, wantOK: true, wantTool: "httpfetch"},
		{name: "duckduckgo_search", key: "duckduckgo_search", merged: nil, wantOK: true, wantTool: "duckduckgo"},
		{name: "arxiv_search", key: "arxiv_search", merged: nil, wantOK: true, wantTool: "arxiv_search"},
		{name: "wikipedia_search", key: "wikipedia_search", merged: nil, wantOK: true, wantTool: "wikipedia"},
		{name: "send_email", key: "send_email", merged: nil, wantOK: true, wantTool: "email"},
		{name: "todo_write", key: "todo_write", merged: nil, wantOK: true, wantTool: "todo"},
		{name: "await_user_reply", key: "await_user_reply", merged: nil, wantOK: true, wantTool: "await_user_reply"},
		{name: "gemini_web_fetch default", key: "gemini_web_fetch", merged: nil, wantOK: true, wantTool: "geminifetch"},
		{name: "gemini_web_fetch with model", key: "gemini_web_fetch", merged: map[string]any{"gemini_model": "gemini-2"}, wantOK: true, wantTool: "geminifetch"},
		{name: "google_search default", key: "google_search", merged: nil, wantOK: true, wantTool: "google_search"},
		{name: "google_search with keys", key: "google_search", merged: map[string]any{"api_key": "k1", "cx": "c1"}, wantOK: true, wantTool: "google_search"},
		{name: "claude_code default", key: "claude_code", merged: nil, wantOK: true, wantTool: "claudecode"},
		{name: "claude_code with dir", key: "claude_code", merged: map[string]any{"base_dir": "/ws"}, wantOK: true, wantTool: "claudecode"},
		{name: "read_multiple_files", key: "read_multiple_files", merged: nil, wantOK: true, wantTool: "file"},
		{name: "save_file", key: "save_file", merged: nil, wantOK: true, wantTool: "file"},
		{name: "list_file", key: "list_file", merged: nil, wantOK: true, wantTool: "file"},
		{name: "search_file", key: "search_file", merged: nil, wantOK: true, wantTool: "file"},
		{name: "search_content", key: "search_content", merged: nil, wantOK: true, wantTool: "file"},
		{name: "replace_content", key: "replace_content", merged: nil, wantOK: true, wantTool: "file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := AssemblyForCatalogKey(tt.key, tt.merged, nil)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v wantOK=%v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if len(cfg.EnabledTools) == 0 && len(cfg.CustomTools) == 0 {
				t.Fatal("expected at least one enabled tool or custom tool")
			}
			if tt.wantTool != "" && len(cfg.EnabledTools) > 0 && cfg.EnabledTools[0] != tt.wantTool {
				t.Fatalf("EnabledTools[0]=%q want %q", cfg.EnabledTools[0], tt.wantTool)
			}
		})
	}
}

func TestAssemblyForCatalogKey_googleSearchKeys(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("google_search", map[string]any{
		"google_api_key": "gkey",
		"search_engine_id": "scx",
	}, nil)
	if !ok {
		t.Fatal("expected ok")
	}
	if cfg.GoogleAPIKey != "gkey" {
		t.Fatalf("GoogleAPIKey=%q want gkey", cfg.GoogleAPIKey)
	}
	if cfg.GoogleCX != "scx" {
		t.Fatalf("GoogleCX=%q want scx", cfg.GoogleCX)
	}
}

func TestAssemblyForCatalogKey_claudeCodeDir(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("claude_code", map[string]any{
		"claude_code_dir": "/my/dir",
	}, nil)
	if !ok {
		t.Fatal("expected ok")
	}
	if cfg.ClaudeCodeDir != "/my/dir" {
		t.Fatalf("ClaudeCodeDir=%q want /my/dir", cfg.ClaudeCodeDir)
	}
}

func TestAssemblyForCatalogKey_geminiModel(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("gemini_web_fetch", map[string]any{
		"model": "gemini-pro",
	}, nil)
	if !ok {
		t.Fatal("expected ok")
	}
	if cfg.GeminiModel != "gemini-pro" {
		t.Fatalf("GeminiModel=%q want gemini-pro", cfg.GeminiModel)
	}
}

func TestAssemblyForCatalogKey_filesystemDir(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		merged  map[string]any
		wantDir string
	}{
		{name: "filesystem_dir", key: "read_file", merged: map[string]any{"filesystem_dir": "/fs"}, wantDir: "/fs"},
		{name: "base_dir", key: "read_file", merged: map[string]any{"base_dir": "/bd"}, wantDir: "/bd"},
		{name: "working_dir", key: "read_file", merged: map[string]any{"working_dir": "/wd"}, wantDir: "/wd"},
		{name: "root_dir", key: "read_file", merged: map[string]any{"root_dir": "/rd"}, wantDir: "/rd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := AssemblyForCatalogKey(tt.key, tt.merged, nil)
			if !ok {
				t.Fatal("expected ok")
			}
			if cfg.FilesystemDir != tt.wantDir {
				t.Fatalf("FilesystemDir=%q want %q", cfg.FilesystemDir, tt.wantDir)
			}
		})
	}
}

func TestAssemblyForCatalogKey_shellExecDir(t *testing.T) {
	tests := []struct {
		name    string
		merged  map[string]any
		wantDir string
	}{
		{name: "base_dir", merged: map[string]any{"base_dir": "/bd"}, wantDir: "/bd"},
		{name: "shell_root", merged: map[string]any{"shell_root": "/sr"}, wantDir: "/sr"},
		{name: "filesystem_dir", merged: map[string]any{"filesystem_dir": "/fs"}, wantDir: "/fs"},
		{name: "working_dir", merged: map[string]any{"working_dir": "/wd"}, wantDir: "/wd"},
		{name: "root_dir", merged: map[string]any{"root_dir": "/rd"}, wantDir: "/rd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, ok := AssemblyForCatalogKey("shell_exec", tt.merged, nil)
			if !ok {
				t.Fatal("expected ok")
			}
			if cfg.ShellExecDir != tt.wantDir {
				t.Fatalf("ShellExecDir=%q want %q", cfg.ShellExecDir, tt.wantDir)
			}
		})
	}
}

func TestExecute_validation(t *testing.T) {
	tests := []struct {
		name    string
		tool    CatalogTool
		wantErr bool
	}{
		{
			name:    "empty key",
			tool:    CatalogTool{Key: ""},
			wantErr: true,
		},
		{
			name:    "whitespace key",
			tool:    CatalogTool{Key: "   "},
			wantErr: true,
		},
		{
			name:    "mcp source",
			tool:    CatalogTool{Key: "some_tool", Source: "mcp"},
			wantErr: true,
		},
		{
			name:    "MCP source case insensitive",
			tool:    CatalogTool{Key: "some_tool", Source: "MCP"},
			wantErr: true,
		},
		{
			name:    "knowledge_search key",
			tool:    CatalogTool{Key: "knowledge_search"},
			wantErr: true,
		},
		{
			name:    "call_agent key",
			tool:    CatalogTool{Key: "call_agent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Execute(context.Background(), tt.tool, "{}", 30, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestMergeJSONInto(t *testing.T) {
	dst := map[string]any{"a": "1"}
	mergeJSONInto(dst, `{"b":"2"}`)
	if dst["a"] != "1" || dst["b"] != "2" {
		t.Fatalf("mergeJSONInto result=%v", dst)
	}

	mergeJSONInto(dst, `{bad`)
	if len(dst) != 2 {
		t.Fatalf("invalid json should not modify dst, got %v", dst)
	}

	mergeJSONInto(dst, "")
	if len(dst) != 2 {
		t.Fatalf("empty string should not modify dst, got %v", dst)
	}

	mergeJSONInto(dst, "{}")
	if len(dst) != 2 {
		t.Fatalf("empty object should not modify dst, got %v", dst)
	}
}

func TestResult_fields(t *testing.T) {
	r := Result{
		Status:        "success",
		ResultPreview: "preview",
		ErrorMessage:  "",
		DurationMS:    100,
	}
	if r.Status != "success" {
		t.Fatalf("Status=%q want success", r.Status)
	}
	if r.DurationMS != 100 {
		t.Fatalf("DurationMS=%d want 100", r.DurationMS)
	}
}

func TestCatalogTool_fields(t *testing.T) {
	ct := CatalogTool{
		Key:               "my_tool",
		Source:            "builtin",
		ConfigJSON:        `{"a":"1"}`,
		DefaultConfigJSON: `{"b":"2"}`,
		MetadataJSON:      `{"c":"3"}`,
	}
	if ct.Key != "my_tool" {
		t.Fatalf("Key=%q want my_tool", ct.Key)
	}
}

func TestPreviewValue_truncation(t *testing.T) {
	longStr := ""
	for i := 0; i < 5000; i++ {
		longStr += "x"
	}
	got := previewValue(longStr)
	if len(got) > 4000 {
		t.Fatalf("previewValue result len=%d want <=4000", len(got))
	}
}

func TestAssemblyForCatalogKey_googleSearchAlternativeKeys(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("google_search", map[string]any{
		"api_key":    "ak",
		"engine_id":  "ei",
		"google_cx":  "gcx",
	}, nil)
	if !ok {
		t.Fatal("expected ok")
	}
	if cfg.GoogleAPIKey != "ak" {
		t.Fatalf("GoogleAPIKey=%q want ak", cfg.GoogleAPIKey)
	}
	if cfg.GoogleCX != "ei" {
		t.Fatalf("GoogleCX=%q want ei (engine_id has priority)", cfg.GoogleCX)
	}
}

func TestOpenAPISpecFromCatalogTool_dataFallback(t *testing.T) {
	tool := CatalogTool{
		Key:          "myapi",
		MetadataJSON: `{"openapi_spec_data":"","spec_data":"{\"openapi\":\"3.0\"}"}`,
	}
	spec, ok := openAPISpecFromCatalogTool(tool)
	if !ok {
		t.Fatal("expected ok")
	}
	if string(spec.SpecData) != `{"openapi":"3.0"}` {
		t.Fatalf("SpecData=%q want openapi 3.0", string(spec.SpecData))
	}
}

func TestAssemblyForCatalogKey_claudeCodeAlternativeDirKeys(t *testing.T) {
	cfg, ok := AssemblyForCatalogKey("claude_code", map[string]any{
		"working_dir": "/alt/dir",
	}, nil)
	if !ok {
		t.Fatal("expected ok")
	}
	if cfg.ClaudeCodeDir != "/alt/dir" {
		t.Fatalf("ClaudeCodeDir=%q want /alt/dir", cfg.ClaudeCodeDir)
	}
}

func TestExecute_unsupportedTool(t *testing.T) {
	_, err := Execute(context.Background(), CatalogTool{Key: "totally_unknown_tool"}, "{}", 30, nil)
	if err == nil {
		t.Fatal("expected error for unsupported tool")
	}
}

func TestDefaultConstants(t *testing.T) {
	if DefaultTimeoutSec != 30 {
		t.Fatalf("DefaultTimeoutSec=%d want 30", DefaultTimeoutSec)
	}
	if MaxTimeoutSec != 120 {
		t.Fatalf("MaxTimeoutSec=%d want 120", MaxTimeoutSec)
	}
}
