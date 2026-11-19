// 审计导出快照/并发/转义测试(自 ceo-dashboard/internal/store/export_test.go
// 移植;WAL 文件库 + 只读事务快照语义不变,负载用例种子改走 Ent bulk)。
package store

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/data/ent"
)

type gatedWriter struct {
	bytes.Buffer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGatedWriter() *gatedWriter {
	return &gatedWriter{started: make(chan struct{}), release: make(chan struct{})}
}

func (w *gatedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.Buffer.Write(p)
}

type exportResult struct {
	rows      int
	truncated bool
	err       error
}

func openExportTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "export.db")
	s, db := openFileStore(t, path)
	t.Cleanup(func() { s.client.Close(); db.Close() })
	return s, path
}

// 快照语义:导出建立快照后的新提交不得出现在 CSV 中。
func TestAuditExportSnapshotExcludesLaterCommit(t *testing.T) {
	s, _ := openExportTestStore(t)
	ctx := context.Background()
	for i := 0; i < 501; i++ {
		if err := s.AddAudit(ctx, "before", "replay", fmt.Sprintf("dl-%d", i), true, "snapshot"); err != nil {
			t.Fatal(err)
		}
	}
	out := newGatedWriter()
	done := make(chan exportResult, 1)
	go func() {
		rows, truncated, err := s.ExportAudit(ctx, out, AuditExportFilter{
			From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100000,
		})
		done <- exportResult{rows: rows, truncated: truncated, err: err}
	}()
	<-out.started
	if err := s.AddAudit(ctx, "after", "replay", "dl-2", true, "must-not-appear"); err != nil {
		t.Fatal(err)
	}
	close(out.release)
	result := <-done
	if result.err != nil || result.rows != 501 {
		t.Fatalf("快照只应包含导出前提交: rows=%d err=%v", result.rows, result.err)
	}
	records, err := csv.NewReader(bytes.NewReader(out.Bytes())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 502 || records[1][1] == "" || strings.Contains(out.String(), "must-not-appear") {
		t.Fatalf("快照包含了导出后的提交或 CSV 非法: %q", out.String())
	}
	wantHeader := []string{"id", "at", "actor", "action", "target", "ok", "detail"}
	for i := range wantHeader {
		if records[0][i] != wantHeader[i] {
			t.Fatalf("CSV 列表头不符: %v", records[0])
		}
	}
}

// 只读导出(WAL 快照)不得阻塞事件写入。
func TestAuditExportReadOnlyDoesNotBlockWriter(t *testing.T) {
	s, _ := openExportTestStore(t)
	ctx := context.Background()
	if err := s.AddAudit(ctx, "boss", "replay", "dl-1", true, "snapshot"); err != nil {
		t.Fatal(err)
	}
	out := newGatedWriter()
	done := make(chan exportResult, 1)
	go func() {
		rows, truncated, err := s.ExportAudit(ctx, out, AuditExportFilter{
			From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100000,
		})
		done <- exportResult{rows: rows, truncated: truncated, err: err}
	}()
	<-out.started
	writeDone := make(chan error, 1)
	go func() {
		_, err := s.InsertEvent(ctx, &Event{
			UID: "hcf:during-export", Platform: "hcf", Subject: "trade.order.v1", SubjectKey: "trade.order",
			EventID: "during-export", ReceivedAt: time.Now(), Payload: strings.Repeat("x", 1024),
		})
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			close(out.release)
			<-done
			t.Fatalf("只读导出期间事件写入失败: %v", err)
		}
	case <-time.After(2 * time.Second):
		close(out.release)
		<-done
		t.Fatal("只读快照导出阻塞了事件写入")
	}
	close(out.release)
	if result := <-done; result.err != nil {
		t.Fatal(result.err)
	}
}

// 公式前缀转义(防表格软件执行审计文本)。
func TestAuditExportEscapesSpreadsheetFormulas(t *testing.T) {
	s, _ := openExportTestStore(t)
	ctx := context.Background()
	if err := s.AddAudit(ctx, "=HYPERLINK(\"https://invalid\")", "+SUM(1,2)", "@cmd", true, "-2+3"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, _, err := s.ExportAudit(ctx, &out, AuditExportFilter{
		From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100000,
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&out).ReadAll()
	if err != nil || len(records) != 2 {
		t.Fatalf("CSV 读取失败: records=%v err=%v", records, err)
	}
	for _, column := range []int{2, 3, 4, 6} {
		cell := records[1][column]
		if len(cell) == 0 || cell[0] != '\'' {
			t.Fatalf("公式前缀必须转义, column=%d cell=%q", column, cell)
		}
	}
}

// 评审 2026-11-09 D4:字节预算钳制——触预算即截断(标记行收尾),不超预算上限;
// 预算充足时全量导出且不截断。10 万行 × 长 detail 必超 gRPC 4MiB 默认上限,
// 行数钳制管不住字节,故独立字节预算(生产常量 3MiB,此处小预算加速)。
func TestAuditExportByteBudgetTruncates(t *testing.T) {
	s, _ := openExportTestStore(t)
	ctx := context.Background()
	base := time.Now().Add(-time.Minute).UTC() // 与 AddAudit 写入口径一致(UTC);sqlite 字典序比较要求两侧同形
	// 40 行 × ~1KB detail:小预算 4KB 必然中途截断。
	bulk := make([]*ent.CeoboardAuditCreate, 0, 40)
	for i := 0; i < 40; i++ {
		bulk = append(bulk, s.client.CeoboardAudit.Create().
			SetAt(base.Add(time.Duration(i)*time.Nanosecond)).
			SetActor("budget").
			SetAction("replay").
			SetTarget(fmt.Sprintf("dl-%d", i)).
			SetOk(true).
			SetDetail(strings.Repeat("d", 1024)))
	}
	if err := s.client.CeoboardAudit.CreateBulk(bulk...).Exec(ctx); err != nil {
		t.Fatal(err)
	}

	const budget = 4096
	var out bytes.Buffer
	rows, truncated, err := s.exportAudit(ctx, &out, AuditExportFilter{
		From: base.Add(-time.Second), To: base.Add(time.Minute), Limit: 100000,
	}, budget)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("超预算必须报告截断")
	}
	if rows <= 0 || rows >= 40 {
		t.Fatalf("截断导出行数应落在 (0,40),实际 %d", rows)
	}
	if out.Len() > budget {
		t.Fatalf("输出 %d 字节超预算 %d(估算上界判定失真)", out.Len(), budget)
	}
	if !strings.Contains(out.String(), "# truncated:") {
		t.Fatal("截断导出必须带 # truncated 标记行")
	}
	// 截断标记之后不得再有数据行。
	tail := out.String()
	markAt := strings.Index(tail, "# truncated:")
	if strings.Count(tail[markAt:], "\n") > 1 {
		t.Fatalf("截断标记后不得再有内容: %q", tail[markAt:])
	}

	// 预算充足:全量 40 行,不截断、无标记。
	var full bytes.Buffer
	rows, truncated, err = s.exportAudit(ctx, &full, AuditExportFilter{
		From: base.Add(-time.Second), To: base.Add(time.Minute), Limit: 100000,
	}, 1<<20)
	if err != nil || truncated || rows != 40 {
		t.Fatalf("预算充足应全量不截断: rows=%d truncated=%v err=%v", rows, truncated, err)
	}
	if strings.Contains(full.String(), "# truncated:") {
		t.Fatal("未截断的导出不得含截断标记")
	}
}

// 显式负载测试:10 万行审计导出期间,事件写入 P95 不得超预算。
func TestAuditExport100kWriteP95(t *testing.T) {
	if os.Getenv("CEOD_LOAD_TEST") != "1" {
		t.Skip("显式负载测试")
	}
	s, _ := openExportTestStore(t)
	ctx := context.Background()
	const auditRows = 100000
	// 必须 UTC:SQLite 方言把 at 存成带本地偏移的文本,导出边界按 UTC 绑定,
	// 用本地时区种子会因字符串比较失败导致导出 0 行(PG timestamptz 无此问题)。
	base := time.Now().UTC().Add(-time.Minute)
	// Ent bulk 分批种子(每批 1000)
	for start := 0; start < auditRows; start += 1000 {
		end := start + 1000
		if end > auditRows {
			end = auditRows
		}
		bulk := make([]*ent.CeoboardAuditCreate, 0, end-start)
		for i := start; i < end; i++ {
			bulk = append(bulk, s.client.CeoboardAudit.Create().
				SetAt(base.Add(time.Duration(i)*time.Nanosecond)).
				SetActor("load").
				SetAction("replay").
				SetTarget(fmt.Sprintf("dl-%d", i)).
				SetOk(true).
				SetDetail("seed"))
		}
		if err := s.client.CeoboardAudit.CreateBulk(bulk...).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}

	out := newGatedWriter()
	exportDone := make(chan exportResult, 1)
	exportCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	go func() {
		rows, truncated, err := s.ExportAudit(exportCtx, out, AuditExportFilter{
			From: base.Add(-time.Second), To: base.Add(2 * time.Minute), Limit: auditRows,
		})
		exportDone <- exportResult{rows: rows, truncated: truncated, err: err}
	}()
	<-out.started

	const writes = 1000
	payload := strings.Repeat("x", 1024)
	ticker := time.NewTicker(50 * time.Millisecond) // 固定 20 writes/s
	latencies := make([]time.Duration, 0, writes)
	var writeErr error
	for i := 0; i < writes; i++ {
		<-ticker.C
		started := time.Now()
		_, err := s.InsertEvent(ctx, &Event{
			UID: fmt.Sprintf("hcf:load-%d", i), Platform: "hcf", Subject: "trade.order.v1", SubjectKey: "trade.order",
			EventID: fmt.Sprintf("load-%d", i), ReceivedAt: time.Now(), Payload: payload,
		})
		latencies = append(latencies, time.Since(started))
		if err != nil {
			writeErr = err
			break
		}
	}
	ticker.Stop()
	close(out.release)
	result := <-exportDone
	if writeErr != nil {
		t.Fatalf("导出期间事件写入失败: %v", writeErr)
	}
	// 导出受双钳制:行数 ≤10 万、字节 ≤3MiB。10 万行必然先触字节预算而截断,
	// 故此处只断言"导出确实产出且无错",截断由 truncated 标记并经日志留痕。
	if result.err != nil || result.rows == 0 {
		t.Fatalf("10 万行导出失败: rows=%d truncated=%v err=%v", result.rows, result.truncated, result.err)
	}
	t.Logf("export_rows=%d truncated=%v", result.rows, result.truncated)
	if len(latencies) < writes {
		t.Fatalf("写入样本不足: %d", len(latencies))
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[(len(latencies)*95+99)/100-1]
	t.Logf("write_p95_ms=%.3f samples=%d", float64(p95)/float64(time.Millisecond), len(latencies))
	if p95 > 250*time.Millisecond {
		t.Fatalf("导出期间事件写入 P95 超预算: %s", p95)
	}
}
