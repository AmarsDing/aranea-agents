// 评审 2026-11-09 D7 测试:Scan/rows.Err 错误浮出水面(残缺概览不再 exit 0),
// 单节失败不阻断其余节;resolveDSN 的 flag/环境优先级。存储走 modernc sqlite
// (与 storetest 同款纯 Go 驱动,无 cgo;PG 语法子集 GROUP BY 1,2 / ORDER BY 3
// 在 sqlite 可用,TIMESTAMP 列由驱动往返 time.Time)。
package main

import (
	"bytes"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openCheckDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:dbcheck-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		`CREATE TABLE ceoboard_events(event_uid TEXT PRIMARY KEY, platform TEXT, subject_key TEXT, received_at TIMESTAMP)`,
		`CREATE TABLE ceoboard_latency_samples(metric_id TEXT, latency_ms INTEGER)`,
		`CREATE TABLE ceoboard_audit(at TIMESTAMP, actor TEXT, action TEXT, ok BOOLEAN, detail TEXT)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func seedClean(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO ceoboard_events VALUES('hcf:a','hcf','order.v1',?)`, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ceoboard_latency_samples VALUES('m1', 12)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO ceoboard_audit VALUES(?,'alice','replay',true,'ok')`, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestRunCleanDBReturnsZero(t *testing.T) {
	db := openCheckDB(t)
	seedClean(t, db)
	var out, errb bytes.Buffer
	if code := run(db, &out, &errb); code != 0 {
		t.Fatalf("干净库应 exit 0,实际 %d stderr=%s\n%s", code, errb.String(), out.String())
	}
	for _, want := range []string{"ceoboard_events 分布", "ceoboard_latency_samples", "ceoboard_audit", "hcf"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("输出缺 %q:\n%s", want, out.String())
		}
	}
}

// 毒值注入 received_at(非时间文本):events 段 Scan/迭代失败必须上浮为
// 节错误 → run exit 1,且其余两节照常输出(诊断工具看全三段)。
func TestRunScanFailureSurfacesAndContinues(t *testing.T) {
	db := openCheckDB(t)
	seedClean(t, db)
	if _, err := db.Exec(`INSERT INTO ceoboard_events VALUES('hcf:bad','hcf','order.v1','not-a-time')`); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run(db, &out, &errb); code != 1 {
		t.Fatalf("Scan 失败必须 exit 1,实际 %d", code)
	}
	if !strings.Contains(errb.String(), "ceoboard_events 段失败") {
		t.Fatalf("失败节必须 stderr 留痕: %s", errb.String())
	}
	if !strings.Contains(out.String(), "ceoboard_latency_samples") || !strings.Contains(out.String(), "ceoboard_audit") {
		t.Fatalf("其余节必须照常输出:\n%s", out.String())
	}
}

func TestResolveDSNPriority(t *testing.T) {
	t.Setenv("ARANEA_PG_DSN", "env-dsn")
	if got := resolveDSN("flag-dsn"); got != "flag-dsn" {
		t.Fatalf("flag 优先于环境: %q", got)
	}
	if got := resolveDSN(""); got != "env-dsn" {
		t.Fatalf("环境兜底: %q", got)
	}
	t.Setenv("ARANEA_PG_DSN", "")
	if got := resolveDSN(""); got != "" {
		t.Fatalf("皆空应空串(调用方 exit 2): %q", got)
	}
}
