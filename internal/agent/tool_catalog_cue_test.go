package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/tools/deferred"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// P1-4：catalog cue 语义预激活——hook 按当前用户 query 渲染 Top-N 推荐区；
// 无匹配时输出必须与静态版字节一致（无 query 场景保持确定性）。

func toolCatalogCueTestHook(t *testing.T, catalog []deferred.DeferredToolEntry) callbacks.Callback {
	t.Helper()
	deps := TRPCBuilderDeps{}.WithDeferredManager(deferred.NewDeferredToolManager(catalog))
	hook := newToolCatalogCueBeforeHook(deps)
	if hook == nil {
		t.Fatal("expected non-nil hook for non-empty catalog")
	}
	return hook
}

func runCatalogCueHook(t *testing.T, hook callbacks.Callback, userText string) string {
	t.Helper()
	bm, ok := hook.(callbacks.BeforeModelHook)
	if !ok {
		t.Fatal("hook must implement BeforeModelHook")
	}
	messages := []trpcmodel.Message{{Role: trpcmodel.RoleUser, Content: userText}}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: messages}}
	if _, err := bm.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("HandleBeforeModel: %v", err)
	}
	if len(args.Request.Messages) != 2 {
		t.Fatalf("expected cue appended (2 messages), got %d", len(args.Request.Messages))
	}
	return args.Request.Messages[1].Content
}

func TestToolCatalogCueHook_NilManager(t *testing.T) {
	if hook := newToolCatalogCueBeforeHook(TRPCBuilderDeps{}); hook != nil {
		t.Error("nil DeferredManager must yield nil hook")
	}
}

func TestToolCatalogCueHook_RecommendsMatchingTools(t *testing.T) {
	catalog := []deferred.DeferredToolEntry{
		{Name: "file_save_file", BaseName: "save_file", Description: "Save content to a file", Category: "file"},
		{Name: "shell_exec", BaseName: "shell_exec", Description: "Execute shell commands", Category: "runtime"},
	}
	hook := toolCatalogCueTestHook(t, catalog)
	cue := runCatalogCueHook(t, hook, "please save this to a file")
	if !strings.Contains(cue, "Recommended") {
		t.Error("cue must contain Recommended section for matching query")
	}
	if !strings.Contains(cue, "file_save_file") {
		t.Error("cue must recommend file_save_file")
	}
}

func TestToolCatalogCueHook_NoMatchKeepsStatic(t *testing.T) {
	catalog := []deferred.DeferredToolEntry{
		{Name: "file_save_file", BaseName: "save_file", Description: "Save content to a file", Category: "file"},
	}
	hook := toolCatalogCueTestHook(t, catalog)
	cue := runCatalogCueHook(t, hook, "zzz qqq nothing relevant")
	if strings.Contains(cue, "Recommended") {
		t.Error("no-match query must not add Recommended section")
	}
	if want := toolCatalogCueMarker + deferred.RenderCatalogCue(catalog); cue != want {
		t.Errorf("no-match cue must equal static render（含段标记，包A A1）.\nwant:\n%s\ngot:\n%s", want, cue)
	}
}

func TestToolCatalogCueHook_NilRequest(t *testing.T) {
	hook := toolCatalogCueTestHook(t, []deferred.DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	})
	bm := hook.(callbacks.BeforeModelHook)
	if _, err := bm.HandleBeforeModel(context.Background(), &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatalf("nil request must not error: %v", err)
	}
}
