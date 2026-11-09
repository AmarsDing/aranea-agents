// 文本消毒与毒消息治理(评审 2026-11-09 H3):
//   - PG TEXT 列拒收非法 UTF-8 与 NUL(SQLSTATE 22021),消毒在写入边界统一
//     完成(事件/死信),消毒后同一原始键恒映射到同一建库键;
//   - 消毒后仍被拒收的持久性数据错误(SQLSTATE class 22)判为毒记录:spool
//     导入时单条隔离进 <段>.poison 后继续,不头阻塞整段。
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

func TestSanitizeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"普通英文", "abcXYZ123", "abcXYZ123"},
		{"中文原样", "事件·信号·对账", "事件·信号·对账"},
		{"空串", "", ""},
		{"NUL 替换", "a\x00b", "a�b"},
		{"非法 UTF-8 替换", "a\xff\xf0b", "a�b"},
		{"混合污染", "\x00中\xff文\x00", "�中�文�"},
		{"多字节合法序列不拆", "日本語—ok", "日本語—ok"},
	}
	for _, c := range cases {
		if got := sanitizeText(c.in); got != c.want {
			t.Fatalf("%s: sanitizeText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// 事件落库:消毒后的 uid 可检索;payload 无 NUL/非法 UTF-8,消毒形态入库。
func TestInsertEventSanitizes(t *testing.T) {
	st := openTest(t)
	rawUID := "u\x00id\xff"
	ev := testEvent(rawUID)
	ev.Payload = "p\x00a\xffy"
	inserted, err := st.InsertEvent(context.Background(), ev)
	if err != nil {
		t.Fatalf("消毒后落库不应失败: %v", err)
	}
	if !inserted {
		t.Fatal("首次写入应为新插入")
	}
	stored, err := st.GetEvent(context.Background(), sanitizeText(rawUID))
	if err != nil {
		t.Fatalf("消毒 uid 应可检索: %v", err)
	}
	if stored.Payload != sanitizeText("p\x00a\xffy") {
		t.Fatalf("payload 应为消毒形态,实际 %q", stored.Payload)
	}
	if strings.IndexByte(stored.Payload, 0) >= 0 {
		t.Fatal("payload 不应含 NUL")
	}
	if strings.Contains(stored.Payload, "�") == false {
		t.Fatal("污染字节应替换为 U+FFFD")
	}
}

// 死信查重键先消毒再比对:同一原始 refKey 的两次重试幂等返回同一 id
// (消毒前写/查键不一致会让每次都插新行)。
func TestAddDeadLetterSanitizedDedup(t *testing.T) {
	st := openTest(t)
	refKey := "rk\x00\xff"
	id1, err := st.AddDeadLetter(context.Background(), "webhook", "hcf", refKey, "p\x01", "r\x00")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := st.AddDeadLetter(context.Background(), "webhook", "hcf", refKey, "p\x01", "r\x00")
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("同一原始 refKey 消毒后应幂等同 id,实际 %d / %d", id1, id2)
	}
	// 显式消毒形态的键也应命中同一行(键一致性)。
	id3, err := st.AddDeadLetter(context.Background(), "webhook", "hcf", sanitizeText(refKey), "p", "r")
	if err != nil {
		t.Fatal(err)
	}
	if id3 != id1 {
		t.Fatalf("消毒形态键应命中同一死信,实际 %d / %d", id3, id1)
	}
}

func TestIsPoisonDataErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"22021 非法 UTF-8", &pq.Error{Code: "22021"}, true},
		{"22001 值超长", &pq.Error{Code: "22001"}, true},
		{"23505 唯一冲突非数据异常", &pq.Error{Code: "23505"}, false},
		{"08006 连接类非数据异常", &pq.Error{Code: "08006"}, false},
		{"包装后仍可识别", fmt.Errorf("事件落库: %w", &pq.Error{Code: "22012"}), true},
		{"普通错误", errors.New("boom"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isPoisonDataErr(c.err); got != c.want {
			t.Fatalf("%s: isPoisonDataErr = %v, want %v", c.name, got, c.want)
		}
	}
}

// 毒记录隔离:append 语义(JSONL,0600),可解析回原记录。
func TestAppendPoisonRecords(t *testing.T) {
	dir := t.TempDir()
	claim := filepath.Join(dir, "events-x.jsonl.importing")
	e1 := testEvent("p1")
	e2 := testEvent("p2")
	recs := []SpoolRecord{
		{Event: *e1, Reason: "22021", SpooledAt: time.Now().UTC()},
		{Event: *e2, Reason: "22001", SpooledAt: time.Now().UTC()},
	}
	path, err := appendPoisonRecords(claim, recs)
	if err != nil {
		t.Fatal(err)
	}
	if path != claim+".poison" {
		t.Fatalf("隔离文件名应为 <段>.poison: %s", path)
	}
	if runtime.GOOS != "windows" { // Windows 不支持 Unix 权限位
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("poison 文件权限应为 0600: %v", fi.Mode().Perm())
		}
	}
	// 再追加一条:文件应累计 3 行。
	if _, err := appendPoisonRecords(claim, recs[:1]); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("poison 应为 3 行(2+1 追加),实际 %d", len(lines))
	}
	var back SpoolRecord
	if err := json.Unmarshal([]byte(lines[0]), &back); err != nil {
		t.Fatalf("poison 行应可解析: %v", err)
	}
	if back.Event.UID != "p1" {
		t.Fatalf("首行事件 uid 应为 p1: %s", back.Event.UID)
	}
}
