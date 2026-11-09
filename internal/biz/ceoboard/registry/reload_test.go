// 热加载原子回退测试(P2 扫漏):任一声明失败 → 旧版整体保留;修复后 → 原子切换。
package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const okWf = `api_version: ceoboard/v1
kind: Workflow
id: wf.a
name: A
biz_line: test
owner: qa
version: 1
permissions: [ceo]
nodes:
  - { id: n1, name: n1, order: 1, enter: "job(state='q')", exit: "job(state='d')" }
`

func TestReload_AtomicRollbackOnFailure(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(okWf), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, rep, err := LoadDir(dir)
	if err != nil || len(rep.Failed) != 0 {
		t.Fatalf("初始加载: %v %+v", err, rep.Failed)
	}

	// 写入一条坏声明 + 一条新的好声明:Reload 应整体回退(坏的不进,新的也不进)
	bad := `api_version: ceoboard/v1
kind: Workflow
id: wf.b
`
	good := `api_version: ceoboard/v1
kind: Workflow
id: wf.c
name: C
biz_line: test
owner: qa
version: 1
permissions: [ceo]
nodes:
  - { id: n1, name: n1, order: 1, enter: "job(state='q')", exit: "job(state='d')" }
`
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte(bad), 0o644)
	os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(good), 0o644)

	rep2, err := reg.Reload(dir)
	if err == nil {
		t.Fatal("有坏声明时 Reload 应报错回退")
	}
	if rep2 == nil || len(rep2.Failed) != 1 {
		t.Fatalf("回退时应返回失败明细,实得 %+v", rep2)
	}
	if reg.Get("wf.a") == nil {
		t.Fatal("回退后旧工作流 wf.a 不应丢失")
	}
	if reg.Get("wf.c") != nil {
		t.Fatal("整体回退:同批好声明 wf.c 也不应生效(不得半新半旧)")
	}

	// 修掉坏声明:Reload 成功,原子切换到新版(含 wf.c)
	os.Remove(filepath.Join(dir, "b.yaml"))
	rep3, err := reg.Reload(dir)
	if err != nil {
		t.Fatalf("修复后 Reload 应成功: %v", err)
	}
	if reg.Get("wf.a") == nil || reg.Get("wf.c") == nil {
		t.Fatalf("切换后应同时含 wf.a/wf.c,实得 %v", reg.List())
	}
	if len(rep3.Loaded) != 2 {
		t.Fatalf("报告应含 2 条 loaded,实得 %v", rep3.Loaded)
	}
}

// 评审 2026-11-09 L4:Reload 报错必须携带文件/记录级细节(热加载调用点只拿到
// err——细节丢失会让"哪条声明改坏了"无从定位);既有契约(回退 + 报告返回)不变。
func TestReload_ErrorCarriesFileAndFieldDetails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(okWf), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, _, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	bad1 := `api_version: ceoboard/v1
kind: Workflow
id: wf.bad1
`
	bad2 := `api_version: ceoboard/v1
kind: Workflow
id: wf.bad2
name: [未闭合
`
	os.WriteFile(filepath.Join(dir, "bad1.yaml"), []byte(bad1), 0o644)
	os.WriteFile(filepath.Join(dir, "bad2.yaml"), []byte(bad2), 0o644)

	rep, err := reg.Reload(dir)
	if err == nil {
		t.Fatal("有坏声明时 Reload 应报错回退")
	}
	msg := err.Error()
	if !strings.Contains(msg, "bad1.yaml") || !strings.Contains(msg, "bad2.yaml") {
		t.Fatalf("错误必须点名失败文件: %q", msg)
	}
	if !strings.Contains(msg, "name") {
		t.Fatalf("错误必须携带字段/校验级细节: %q", msg)
	}
	if !strings.Contains(msg, "回退") {
		t.Fatalf("错误必须保留整体回退语义: %q", msg)
	}
	// 完整明细仍在报告中(错误只带前 8 条摘要)。
	if rep == nil || len(rep.Failed) != 2 {
		t.Fatalf("报告应含 2 条失败明细,实得 %+v", rep)
	}
	if reg.Get("wf.a") == nil {
		t.Fatal("回退后旧工作流不应丢失")
	}
}
