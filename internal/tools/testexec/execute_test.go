package testexec_test

import (
	"context"
	"testing"

	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/testexec"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubTool struct {
	decl *trpctool.Declaration
}

func (s *stubTool) Declaration() *trpctool.Declaration {
	return s.decl
}

type stubCallableTool struct {
	decl *trpctool.Declaration
}

func (s *stubCallableTool) Declaration() *trpctool.Declaration {
	return s.decl
}

func (s *stubCallableTool) Call(_ context.Context, _ []byte) (any, error) {
	return nil, nil
}

type stubToolSet struct {
	nameFn  func() string
	toolsFn func(ctx context.Context) []trpctool.Tool
	closeFn func() error
}

func (s *stubToolSet) Name() string {
	if s.nameFn != nil {
		return s.nameFn()
	}
	return ""
}

func (s *stubToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s.toolsFn != nil {
		return s.toolsFn(ctx)
	}
	return nil
}

func (s *stubToolSet) Close() error {
	if s.closeFn != nil {
		return s.closeFn()
	}
	return nil
}

func TestMatchCallable(t *testing.T) {
	tests := []struct {
		name      string
		toolsList []trpctool.Tool
		toolName  string
		wantOK    bool
		wantName  string
	}{
		{
			name:      "tool found",
			toolsList: []trpctool.Tool{&stubCallableTool{decl: &trpctool.Declaration{Name: "my_tool"}}},
			toolName:  "my_tool",
			wantOK:    true,
			wantName:  "my_tool",
		},
		{
			name:      "tool not found",
			toolsList: []trpctool.Tool{&stubCallableTool{decl: &trpctool.Declaration{Name: "other_tool"}}},
			toolName:  "my_tool",
			wantOK:    false,
		},
		{
			name:      "nil list",
			toolsList: nil,
			toolName:  "my_tool",
			wantOK:    false,
		},
		{
			name: "multiple tools first match returned",
			toolsList: []trpctool.Tool{
				&stubCallableTool{decl: &trpctool.Declaration{Name: "first"}},
				&stubCallableTool{decl: &trpctool.Declaration{Name: "second"}},
			},
			toolName: "first",
			wantOK:   true,
			wantName: "first",
		},
		{
			name: "nil tool in list skipped",
			toolsList: []trpctool.Tool{
				nil,
				&stubCallableTool{decl: &trpctool.Declaration{Name: "my_tool"}},
			},
			toolName: "my_tool",
			wantOK:   true,
			wantName: "my_tool",
		},
		{
			name:      "tool with nil declaration skipped",
			toolsList: []trpctool.Tool{&stubTool{decl: nil}},
			toolName:  "my_tool",
			wantOK:    false,
		},
		{
			name:      "non-callable tool skipped",
			toolsList: []trpctool.Tool{&stubTool{decl: &trpctool.Declaration{Name: "my_tool"}}},
			toolName:  "my_tool",
			wantOK:    false,
		},
		{
			name: "whitespace name trimmed and matched",
			toolsList: []trpctool.Tool{
				&stubCallableTool{decl: &trpctool.Declaration{Name: "  my_tool  "}},
			},
			toolName: "my_tool",
			wantOK:   true,
			wantName: "  my_tool  ",
		},
		{
			name:      "empty list",
			toolsList: []trpctool.Tool{},
			toolName:  "my_tool",
			wantOK:    false,
		},
		{
			name: "callable tool after non-callable match",
			toolsList: []trpctool.Tool{
				&stubTool{decl: &trpctool.Declaration{Name: "my_tool"}},
				&stubCallableTool{decl: &trpctool.Declaration{Name: "my_tool"}},
			},
			toolName: "my_tool",
			wantOK:   true,
			wantName: "my_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callable, ok := testexec.MatchCallable(tt.toolsList, tt.toolName)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v wantOK=%v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				if callable != nil {
					t.Fatal("expected nil callable when ok=false")
				}
				return
			}
			if callable == nil {
				t.Fatal("callable is nil but ok=true")
			}
			d := callable.Declaration()
			if d == nil || d.Name != tt.wantName {
				t.Fatalf("Declaration().Name=%q want %q", d.Name, tt.wantName)
			}
		})
	}
}

func TestFindCallable(t *testing.T) {
	ctx := context.Background()

	myTool := &stubCallableTool{decl: &trpctool.Declaration{Name: "my_tool"}}
	otherTool := &stubCallableTool{decl: &trpctool.Declaration{Name: "other_tool"}}

	tests := []struct {
		name     string
		ts       *tools.AssembledToolsets
		names    []string
		wantErr  bool
		wantName string
	}{
		{
			name:     "found in Tools",
			ts:       &tools.AssembledToolsets{Tools: []tools.Tool{myTool}},
			names:    []string{"my_tool"},
			wantErr:  false,
			wantName: "my_tool",
		},
		{
			name: "found in first ToolSet",
			ts: &tools.AssembledToolsets{
				ToolSets: []tools.ToolSet{
					&stubToolSet{
						nameFn:  func() string { return "set1" },
						toolsFn: func(_ context.Context) []trpctool.Tool { return []trpctool.Tool{myTool} },
					},
				},
			},
			names:    []string{"my_tool"},
			wantErr:  false,
			wantName: "my_tool",
		},
		{
			name: "found in second ToolSet",
			ts: &tools.AssembledToolsets{
				ToolSets: []tools.ToolSet{
					&stubToolSet{
						nameFn:  func() string { return "set1" },
						toolsFn: func(_ context.Context) []trpctool.Tool { return []trpctool.Tool{otherTool} },
					},
					&stubToolSet{
						nameFn:  func() string { return "set2" },
						toolsFn: func(_ context.Context) []trpctool.Tool { return []trpctool.Tool{myTool} },
					},
				},
			},
			names:    []string{"my_tool"},
			wantErr:  false,
			wantName: "my_tool",
		},
		{
			name:     "not found",
			ts:       &tools.AssembledToolsets{Tools: []tools.Tool{otherTool}},
			names:    []string{"my_tool"},
			wantErr:  true,
		},
		{
			name:    "nil AssembledToolsets",
			ts:      nil,
			names:   []string{"my_tool"},
			wantErr: true,
		},
		{
			name: "nil ToolSet in list skipped",
			ts: &tools.AssembledToolsets{
				ToolSets: []tools.ToolSet{
					nil,
					&stubToolSet{
						nameFn:  func() string { return "set2" },
						toolsFn: func(_ context.Context) []trpctool.Tool { return []trpctool.Tool{myTool} },
					},
				},
			},
			names:    []string{"my_tool"},
			wantErr:  false,
			wantName: "my_tool",
		},
		{
			name:     "empty names",
			ts:       &tools.AssembledToolsets{Tools: []tools.Tool{myTool}},
			names:    []string{},
			wantErr:  true,
		},
		{
			name:     "multiple names found by second",
			ts:       &tools.AssembledToolsets{Tools: []tools.Tool{otherTool}},
			names:    []string{"missing", "other_tool"},
			wantErr:  false,
			wantName: "other_tool",
		},
		{
			name:    "empty AssembledToolsets",
			ts:      &tools.AssembledToolsets{},
			names:   []string{"my_tool"},
			wantErr: true,
		},
		{
			name: "Tools takes priority over ToolSets",
			ts: &tools.AssembledToolsets{
				Tools: []tools.Tool{myTool},
				ToolSets: []tools.ToolSet{
					&stubToolSet{
						nameFn:  func() string { return "set1" },
						toolsFn: func(_ context.Context) []trpctool.Tool { return []trpctool.Tool{otherTool} },
					},
				},
			},
			names:    []string{"my_tool"},
			wantErr:  false,
			wantName: "my_tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callable, err := testexec.FindCallable(ctx, tt.ts, tt.names...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if callable == nil {
				t.Fatal("callable is nil but err=nil")
			}
			d := callable.Declaration()
			if d == nil || d.Name != tt.wantName {
				t.Fatalf("Declaration().Name=%q want %q", d.Name, tt.wantName)
			}
		})
	}
}
