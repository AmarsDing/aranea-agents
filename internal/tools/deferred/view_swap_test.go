package deferred

import (
	"context"
	"strings"
	"sync"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// P0-2B 方案B：deferredView 热替换间接层。
// 核心不变式：manager 是稳定句柄，SwapView 原子替换当前视图后，
// filter/tool_search/tool_load/catalog cue 四件套立即同刻看到新目录。

func swapViewTool(name string) trpctool.Tool {
	return &mockTool{name: name}
}

// TestSwapView_CatalogAndToolsSwitch 验证视图切换后目录查询与 schema 服务
// 立即指向新视图，旧目录名不再命中。
func TestSwapView_CatalogAndToolsSwitch(t *testing.T) {
	oldMgr := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "alpha", BaseName: "alpha", Description: "old alpha", Category: "c1"},
	})
	oldMgr.RegisterTool("alpha", swapViewTool("alpha"))

	newMgr := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "beta", BaseName: "beta", Description: "new beta", Category: "c2"},
	})
	newMgr.RegisterTool("beta", swapViewTool("beta"))

	oldMgr.SwapView(newMgr)

	if !oldMgr.IsInCatalog("beta") || oldMgr.IsInCatalog("alpha") {
		t.Fatalf("SwapView 后目录应切到新视图, got names=%v", oldMgr.CatalogNames())
	}
	decl := oldMgr.GetToolDeclaration("beta")
	if decl == nil || decl.Description != "mock beta" {
		t.Fatalf("SwapView 后 schema 来源应为新视图工具, got %+v", decl)
	}
	if got := oldMgr.GetToolDeclaration("alpha"); got != nil {
		t.Fatalf("旧视图工具引用不得再可达, got %+v", got)
	}
	if got := oldMgr.CategoryIndex(); len(got) != 1 || len(got["c2"]) != 1 {
		t.Fatalf("分类索引应来自新视图, got %+v", got)
	}
	_, cue := oldMgr.CatalogSnapshot()
	if cue == "" || !strings.Contains(cue, "beta") || strings.Contains(cue, "alpha") {
		t.Fatalf("静态 cue 应渲染新目录, got:\n%s", cue)
	}
}

// TestSwapView_FilterSwitchesAtomically 验证 ToolFilter 闭包（构建期固化、
// 无法被框架替换）在 SwapView 后按新目录隐藏/放行——方案B的立足点。
func TestSwapView_FilterSwitchesAtomically(t *testing.T) {
	oldMgr := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "alpha", BaseName: "alpha"},
	})
	filter := oldMgr.ToolFilter() // 构建期捕获的同一闭包
	ctx := context.Background()   // 无 invocation → 一切延迟工具未激活

	if filter(ctx, swapViewTool("alpha")) {
		t.Fatal("换面前：目录内未激活工具应被隐藏")
	}
	if !filter(ctx, swapViewTool("unrelated")) {
		t.Fatal("换面前：目录外工具应放行")
	}

	newMgr := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "beta", BaseName: "beta"},
	})
	oldMgr.SwapView(newMgr)

	if !filter(ctx, swapViewTool("alpha")) {
		t.Fatal("换面后：已从目录移除的名字不得再被拦截（fail-open 对普通工具）")
	}
	if filter(ctx, swapViewTool("beta")) {
		t.Fatal("换面后：新目录工具未激活必须立即隐藏（fail-closed 对延迟工具）")
	}
}

// TestSwapView_ConcurrentReaders -race 冒烟：换视图瞬间在途 filter/目录读
// 不得有数据竞争或 panic。
func TestSwapView_ConcurrentReaders(t *testing.T) {
	oldMgr := NewDeferredToolManager([]DeferredToolEntry{{Name: "alpha", BaseName: "alpha"}})
	filter := oldMgr.ToolFilter()
	ctx := context.Background()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					filter(ctx, swapViewTool("alpha"))
					oldMgr.Catalog()
					oldMgr.CatalogSnapshot()
					oldMgr.IsInCatalog("alpha")
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		src := NewDeferredToolManager([]DeferredToolEntry{{Name: "beta", BaseName: "beta"}})
		oldMgr.SwapView(src)
	}
	close(stop)
	wg.Wait()
}

// TestCatalogSnapshot_StaticCuePreRendered 验证静态 cue 随视图预渲染：
// 空目录 → 空 cue；非空目录 → 含工具名。
func TestCatalogSnapshot_StaticCuePreRendered(t *testing.T) {
	empty := NewDeferredToolManager(nil)
	if _, cue := empty.CatalogSnapshot(); cue != "" {
		t.Fatalf("空目录静态 cue 应为空, got %q", cue)
	}
	m := NewDeferredToolManager([]DeferredToolEntry{
		{Name: "web_fetch", BaseName: "web_fetch", Description: "Fetch web content", Category: "web"},
	})
	catalog, cue := m.CatalogSnapshot()
	if len(catalog) != 1 || cue == "" {
		t.Fatalf("非空目录应产出快照目录+cue, got %v / %q", catalog, cue)
	}
	if want := RenderCatalogCue(catalog); cue != want {
		t.Fatalf("预渲染 cue 必须与 RenderCatalogCue 字节一致.\nwant:\n%s\ngot:\n%s", want, cue)
	}
}
