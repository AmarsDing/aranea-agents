// 热加载原子回退测试(P2 扫漏):任一声明失败 → 旧版整体保留;修复后 → 原子切换。
package registry

import (
	"os"
	"path/filepath"
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
