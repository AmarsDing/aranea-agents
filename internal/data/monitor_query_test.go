package data

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// HideLinkedCompletions：服务端排除已落入 Runs 的 runner.completion 行——
// usage_event_id 非空、或 trace_id 命中 monitor_traces（trace_key / metadata trace_id）。
// 前端 Events 历史表恒传 true，保证分页 total 与实际渲染行数一致（ISSUE-007）。
func TestMonitorEventsWhereHideLinkedCompletions(t *testing.T) {
	for _, d := range []Dialect{DialectPostgres, DialectSQLite} {
		where, args := monitorEventsWhere(biz.MonitorEventsQuery{HideLinkedCompletions: true}, d)
		if !strings.Contains(where, "event_key != 'runner.completion'") {
			t.Fatalf("%s: hide_linked_completions should scope exclusion to runner.completion, got %q", d, where)
		}
		if !strings.Contains(where, "usage_event_id") {
			t.Fatalf("%s: hide_linked_completions should check usage_event_id, got %q", d, where)
		}
		if !strings.Contains(where, "NOT EXISTS") || !strings.Contains(where, "monitor_traces") || !strings.Contains(where, "trace_key") {
			t.Fatalf("%s: hide_linked_completions should NOT EXISTS against monitor_traces.trace_key, got %q", d, where)
		}
		if len(args) != 0 {
			t.Fatalf("%s: hide_linked_completions takes no args, got %v", d, args)
		}
	}

	// 默认关闭：不带标志时无排除子句，行为不变。
	where, _ := monitorEventsWhere(biz.MonitorEventsQuery{}, DialectPostgres)
	if strings.Contains(where, "runner.completion") {
		t.Fatalf("empty query should not exclude completions, got %q", where)
	}
}

// ExcludeSystem：未显式按动作过滤时追加 sync.% 排除；显式动作过滤优先（不叠加排除）。
func TestAuditWhereExcludeSystem(t *testing.T) {
	where, args := auditWhere(biz.AuditQuery{ExcludeSystem: true})
	if !strings.Contains(where, "action NOT LIKE ?") {
		t.Fatalf("exclude_system should add action NOT LIKE clause, got %q", where)
	}
	if len(args) != 1 || args[0] != "sync.%" {
		t.Fatalf("args = %v, want [sync.%%]", args)
	}

	// 显式动作过滤时排除不生效，否则选 sync 会得到空结果。
	where, _ = auditWhere(biz.AuditQuery{Action: "sync", ExcludeSystem: true})
	if strings.Contains(where, "NOT LIKE") {
		t.Fatalf("explicit action filter must win over exclude_system, got %q", where)
	}
	if !strings.Contains(where, "action LIKE ?") {
		t.Fatalf("verb filter should prefix-match, got %q", where)
	}

	// 默认行为不变：不带 exclude_system 无排除子句。
	where, _ = auditWhere(biz.AuditQuery{})
	if where != "" {
		t.Fatalf("empty query should have no where, got %q", where)
	}
}

// ExcludeInternal：按 name 列（运行域）排除内部域；Keyword 生成多列 LIKE 组合。
func TestMonitorTracesWhereKeywordAndExcludeInternal(t *testing.T) {
	d := DialectPostgres

	where, args := monitorTracesWhere(biz.MonitorTracesQuery{ExcludeInternal: true}, d, traceWhereOmit{})
	if !strings.Contains(where, "name NOT IN ('system', 'skill')") {
		t.Fatalf("exclude_internal should filter internal domains, got %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("exclude_internal takes no args, got %v", args)
	}

	where, args = monitorTracesWhere(biz.MonitorTracesQuery{Keyword: " gpt "}, d, traceWhereOmit{})
	if !strings.Contains(where, "trace_key LIKE ?") || !strings.Contains(where, "model LIKE ?") {
		t.Fatalf("keyword should search name/trace_key/agent_id/provider/model, got %q", where)
	}
	// 5 裸列 + 4 显示名 EXISTS（agents/sessions/teams/session→agent 回退），与列表名称解析对齐。
	for _, want := range []string{"agents ka", "sessions ks", "teams kt", "ks2 JOIN agents ka2", "display_name LIKE ?", "title LIKE ?"} {
		if !strings.Contains(where, want) {
			t.Fatalf("keyword display-name EXISTS missing %q: %q", want, where)
		}
	}
	if len(args) != 9 {
		t.Fatalf("keyword args = %v, want 9 like patterns", args)
	}
	for _, a := range args {
		if a != "%gpt%" {
			t.Fatalf("keyword should be trimmed and wrapped, got %v", a)
		}
	}

	// 组合：status + exclude_internal + keyword 同时生效。
	where, _ = monitorTracesWhere(biz.MonitorTracesQuery{Status: "error", ExcludeInternal: true, Keyword: "x"}, d, traceWhereOmit{})
	for _, want := range []string{"status = ?", "name NOT IN", "LIKE ?"} {
		if !strings.Contains(where, want) {
			t.Fatalf("combined where missing %q: %q", want, where)
		}
	}
}

// Domain：显式域过滤优先于 ExcludeInternal；omit 标志为聚合查询摘除自身维度。
func TestMonitorTracesWhereDomainAndOmit(t *testing.T) {
	d := DialectPostgres

	// 显式 domain 优先（可查到 system/skill 自身），不再叠加 NOT IN 排除。
	where, args := monitorTracesWhere(biz.MonitorTracesQuery{Domain: "system", ExcludeInternal: true}, d, traceWhereOmit{})
	if !strings.Contains(where, "name = ?") {
		t.Fatalf("explicit domain should add name = ?, got %q", where)
	}
	if strings.Contains(where, "NOT IN") {
		t.Fatalf("explicit domain must win over exclude_internal, got %q", where)
	}
	if len(args) != 1 || args[0] != "system" {
		t.Fatalf("domain args = %v, want [system]", args)
	}

	// omit.status：status 条件被摘除（chip 计数用），其余保留。
	where, args = monitorTracesWhere(biz.MonitorTracesQuery{Status: "error", Keyword: "x"}, d, traceWhereOmit{status: true})
	if strings.Contains(where, "status = ?") {
		t.Fatalf("omit.status should drop status clause, got %q", where)
	}
	if !strings.Contains(where, "LIKE ?") {
		t.Fatalf("omit.status must keep keyword clause, got %q", where)
	}
	_ = args

	// omit.domain：Domain 与 ExcludeInternal 一并摘除。
	where, _ = monitorTracesWhere(biz.MonitorTracesQuery{Domain: "chat", ExcludeInternal: true, Status: "ok"}, d, traceWhereOmit{domain: true})
	if strings.Contains(where, "name = ?") || strings.Contains(where, "NOT IN") {
		t.Fatalf("omit.domain should drop domain clauses, got %q", where)
	}
	if !strings.Contains(where, "status = ?") {
		t.Fatalf("omit.domain must keep status clause, got %q", where)
	}
}

// LIKE 转义：%/_/\ 按字面量匹配，防用户输入注入通配符。
func TestEscapeLikePattern(t *testing.T) {
	want := `50\%\_\\"`
	if got := escapeLikePattern(`50%_\"`); got != want {
		t.Fatalf("escapeLikePattern = %q, want %q", got, want)
	}
}
