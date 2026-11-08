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
	rows int
	err  error
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
		rows, err := s.ExportAudit(ctx, out, AuditExportFilter{
			From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100000,
		})
		done <- exportResult{rows: rows, err: err}
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
		rows, err := s.ExportAudit(ctx, out, AuditExportFilter{
			From: time.Now().Add(-time.Hour), To: time.Now().Add(time.Hour), Limit: 100000,
		})
		done <- exportResult{rows: rows, err: err}
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
	_, err := s.ExportAudit(ctx, &out, AuditExportFilter{
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

// 显式负载测试:10 万行审计导出期间,事件写入 P95 不得超预算。
func TestAuditExport100kWriteP95(t *testing.T) {
	if os.Getenv("CEOD_LOAD_TEST") != "1" {
		t.Skip("显式负载测试")
	}
	s, _ := openExportTestStore(t)
	ctx := context.Background()
	const auditRows = 100000
	base := time.Now().Add(-time.Minute)
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
		rows, err := s.ExportAudit(exportCtx, out, AuditExportFilter{
			From: base.Add(-time.Second), To: base.Add(2 * time.Minute), Limit: auditRows,
		})
		exportDone <- exportResult{rows: rows, err: err}
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
	if result.err != nil || result.rows != auditRows {
		t.Fatalf("10 万行导出失败: rows=%d err=%v", result.rows, result.err)
	}
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
