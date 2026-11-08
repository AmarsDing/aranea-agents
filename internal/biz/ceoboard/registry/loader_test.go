package registry

import (
	"os"
	"path/filepath"
	"testing"
)

const goodYAML = `
api_version: ceoboard/v1
kind: Workflow
id: wf.t1
name: 测试流
biz_line: ecommerce
owner: qa
version: 1
permissions: [ceo]
probe: { type: rest, path: "GET http://127.0.0.1:1/healthz", interval: 30s, stale_after: 90s }
nodes:
  - id: n1
    name: 节点一
    order: 1
    enter: "job(state='queued')"
    exit: "job(state in ('done','failed'))"
    source:
      adapter: rest-poll
      rest_path: "GET http://127.0.0.1:1/x"
    metrics:
      - { id: m1, unit: 个, window: realtime, expr: "gauge(count(state='queued'))", thresholds: { red: ">50" } }
`

const badYAML = `
api_version: ceoboard/v1
kind: Workflow
id: wf.bad
name: 缺权限
biz_line: ecommerce
owner: qa
version: 1
nodes:
  - { id: n1, name: x, order: 1, enter: "job(state='queued')", exit: "job(state='done')" }
`

const pendingYAML = `
api_version: ceoboard/v1
kind: Workflow
id: wf.t2
name: 待补流
biz_line: invest
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: n1
    name: 待补节点
    order: 1
    enter: "job(state='queued')"
    exit: "job(state='done')"
    source:
      adapter: rest-poll
      rest_path: "[待补] 某某路由"
      field_map: { a: "[待补] 字段名", b: ok }
    metrics:
      - { id: m1, unit: "%", window: 5m, expr: "ratio(count(x='1'), count(x='2'))", pending: "[待补] 枚举" }
`

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// 好声明加载成功;坏声明被隔离,不影响好的;报告记录错误。
func TestLoadDir_IsolatesBadDeclaration(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a-good.yaml":    goodYAML,
		"b-bad.yaml":     badYAML,
		"c-pending.yaml": pendingYAML,
	})
	reg, rep, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(rep.Loaded) != 2 {
		t.Fatalf("Loaded=%v, 期望 2 条", rep.Loaded)
	}
	if len(rep.Failed) != 1 || rep.Failed[0].File != "b-bad.yaml" {
		t.Fatalf("Failed=%+v, 期望仅 b-bad.yaml", rep.Failed)
	}
	if reg.Get("wf.t1") == nil || reg.Get("wf.t2") == nil {
		t.Fatal("好工作流未加载")
	}
	if reg.Get("wf.bad") != nil {
		t.Fatal("坏工作流不应加载")
	}
}

// [待补] 元素显式跳过:rest_path 清空+标记、field_map 单条删除、metric.pending 记录。
func TestLoadDir_PendingSkipped(t *testing.T) {
	dir := writeFiles(t, map[string]string{"p.yaml": pendingYAML})
	reg, rep, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	wf := reg.Get("wf.t2")
	if wf == nil {
		t.Fatal("wf.t2 未加载")
	}
	src := wf.Nodes[0].Source
	if !src.Skipped || src.RestPath != "" {
		t.Fatalf("source 应被跳过: %+v", src)
	}
	if _, ok := src.FieldMap["a"]; ok {
		t.Fatal("field_map.a 应被删除")
	}
	if src.FieldMap["b"] != "ok" {
		t.Fatal("field_map.b 应保留")
	}
	if len(rep.Skipped) != 3 { // rest_path + field_map.a + metric.pending
		t.Fatalf("Skipped=%+v, 期望 3 条", rep.Skipped)
	}
	if wf.Nodes[0].Metrics[0].Pending == "" {
		t.Fatal("metric pending 标记应保留(供前端占位)")
	}
}

// 校验器:阈值不可解析 / 算子非法 / 表达式形态非法 均被拒。
func TestValidate_RejectsBadExprAndThreshold(t *testing.T) {
	bad := `
api_version: ceoboard/v1
kind: Workflow
id: wf.x
name: x
biz_line: ecommerce
owner: qa
version: 1
permissions: [ceo]
nodes:
  - id: n1
    name: n1
    order: 1
    enter: "不是表达式"
    exit: "job(state='done')"
    metrics:
      - { id: m1, expr: "select * from t", thresholds: { red: "很多" } }
`
	dir := writeFiles(t, map[string]string{"x.yaml": bad})
	_, rep, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Failed) != 1 {
		t.Fatalf("期望 1 条失败,实际 %+v", rep.Failed)
	}
	joined := ""
	for _, e := range rep.Failed[0].Errors {
		joined += e + "|"
	}
	for _, want := range []string{"enter 表达式形态非法", "算子非法", "不可解析"} {
		if !contains(joined, want) {
			t.Fatalf("错误列表缺少 %q: %s", want, joined)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
