package agent

import (
	"strconv"
	"testing"
	"time"
)

// tool_spill_failed_emit_test.go —— 对抗评审 #14：spillFailedDeduper 惰性
// 清扫边界钉死。清扫触发阈 >512、保留桶 ≥cur-1、旧桶再到达合理重发、
// 同桶重复仍被压制。

func TestSpillFailedDeduper_LazySweepBoundedAndSameBucketSuppressed(t *testing.T) {
	t.Parallel()
	d := &spillFailedDeduper{seen: make(map[string]struct{})}
	cur := time.Now().Unix() / 300

	// 512 键旧桶（cur-10..cur-2 混布）+ 1 键保留桶（cur-1）= 513，
	// 下一次入表必触发清扫。
	oldKeys := make([]string, 0, 512)
	for i := 0; i < 512; i++ {
		bucket := cur - 10 + int64(i%9) // cur-10 .. cur-2，全部 < cur-1
		k := "spill_failed:s" + strconv.Itoa(i) + ":" + strconv.FormatInt(bucket, 10)
		d.seen[k] = struct{}{}
		oldKeys = append(oldKeys, k)
	}
	keep := "spill_failed:keep:" + strconv.FormatInt(cur-1, 10)
	d.seen[keep] = struct{}{}

	fresh := "spill_failed:new:" + strconv.FormatInt(cur, 10)
	if !d.seenOnce(fresh) {
		t.Fatal("first sight must fire")
	}
	// 清扫后表界钉死：只允许 cur-1 保键 + 新入键两条存活。
	if got := len(d.seen); got > 2 {
		t.Fatalf("sweep must be bounded: len=%d, want <=2", got)
	}
	if _, ok := d.seen[keep]; !ok {
		t.Fatal("cur-1 bucket key must survive sweep")
	}
	// 同桶重复仍被压制（清扫不能误放同桶第二次）。
	if d.seenOnce(fresh) {
		t.Fatal("same-bucket duplicate must stay suppressed after sweep")
	}
	// 旧桶键再到达：时间已过属合理重发（观测面原则——桶序由墙壁时间权威，
	// 清扫只为内存有界，不装扮成『已观测过』）。
	if len(oldKeys) > 0 && d.seenOnce(oldKeys[0]) {
		return // 重发成功：正确路径（旧桶已被清扫出表）。
	}
	t.Fatal("ANC-注：旧桶键 dropped 后应允许重发")
}
