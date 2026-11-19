package agg

import (
	"testing"
	"time"
)

func mustParse(t *testing.T, s string) ThreshExpr {
	t.Helper()
	e := ParseThresh(s)
	if e.Op == "" {
		t.Fatalf("阈值解析失败: %q", s)
	}
	return e
}

func TestParseThresh(t *testing.T) {
	e := mustParse(t, "=0 持续30m 且探针活")
	if e.Op != "=" || e.Limit != 0 || e.Dur != 30*time.Minute || !e.NeedProbe {
		t.Fatalf("解析错误: %+v", e)
	}
	e = mustParse(t, "<1 持续30m")
	if e.Op != "<" || e.Limit != 1 || e.Dur != 30*time.Minute || e.NeedProbe {
		t.Fatalf("解析错误: %+v", e)
	}
	e = mustParse(t, ">20")
	if e.Op != ">" || e.Limit != 20 || e.Dur != 0 {
		t.Fatalf("即时阈值解析错误: %+v", e)
	}
	if ParseThresh("胡说八道").Op != "" {
		t.Fatal("不可解析应返回空 Op")
	}
}

// bigStep 放大 MaxStep 到 1h——语义边界测试声明"采样连续",单次 Eval 允许大跨步;
// 时钟漂移防护演练(TestClockDrift_*)必须用默认 2min 才有效。
func bigStep(tr *DurationTracker) { tr.MaxStep = time.Hour }

// 边界 1:条件刚好持续满 30m → 必须跃迁并告警(不允许"差 1s 不报"或"刚过不报")。
func TestDuration_ExactlyAtBoundary(t *testing.T) {
	tr := NewDurationTracker()
	bigStep(tr)
	t0 := time.Now()
	// t0 时刻条件成立(开始计时)
	r := tr.Eval("k", t0, true, 30*time.Minute, 30*time.Minute, false)
	if r.Firing || r.NewAlert {
		t.Fatal("首次成立不应立即告警")
	}
	// t0+29m59s:未达时长,不报
	r = tr.Eval("k", t0.Add(29*time.Minute+59*time.Second), true, 30*time.Minute, 30*time.Minute, false)
	if r.Firing || r.NewAlert {
		t.Fatal("29m59s 不应跃迁")
	}
	// t0+30m:刚好到边界,必须跃迁 + 告警
	r = tr.Eval("k", t0.Add(30*time.Minute), true, 30*time.Minute, 30*time.Minute, false)
	if !r.Firing || !r.NewAlert {
		t.Fatalf("恰好 30m 必须跃迁并告警: %+v", r)
	}
}

// 边界 2:抖动中断——第 29 分钟条件断了一次,计时清零,30m 时刻不报。
func TestDuration_FlappingResets(t *testing.T) {
	tr := NewDurationTracker()
	bigStep(tr)
	t0 := time.Now()
	tr.Eval("k", t0, true, 30*time.Minute, 30*time.Minute, false)
	// 29m 处抖动(条件不成立一次)
	r := tr.Eval("k", t0.Add(29*time.Minute), false, 30*time.Minute, 30*time.Minute, false)
	if r.Firing {
		t.Fatal("抖动时不应 firing")
	}
	// 30m 处恢复成立:重新计时,仍不报
	r = tr.Eval("k", t0.Add(30*time.Minute), true, 30*time.Minute, 30*time.Minute, false)
	if r.Firing || r.NewAlert {
		t.Fatal("抖动后计时未清零,误报")
	}
	// 再持续满 30m(t0+60m)才报
	r = tr.Eval("k", t0.Add(60*time.Minute), true, 30*time.Minute, 30*time.Minute, false)
	if !r.Firing || !r.NewAlert {
		t.Fatal("重新持续满 30m 应跃迁")
	}
}

// 边界 3:恢复——firing 后条件不再成立,状态回落,且不产生"恢复告警"(静默回落,健康度回绿)。
func TestDuration_Recovery(t *testing.T) {
	tr := NewDurationTracker()
	bigStep(tr)
	t0 := time.Now()
	tr.Eval("k", t0, true, time.Minute, 30*time.Minute, false)
	r := tr.Eval("k", t0.Add(time.Minute), true, time.Minute, 30*time.Minute, false)
	if !r.Firing {
		t.Fatal("应已 firing")
	}
	r = tr.Eval("k", t0.Add(2*time.Minute), false, time.Minute, 30*time.Minute, false)
	if r.Firing || !r.Recovered || r.NewAlert {
		t.Fatalf("恢复语义错误: %+v", r)
	}
}

// 告警去重:firing 持续期间,30m 内不重复告警;超过 30m 续报一次。
func TestDuration_AlertDedup(t *testing.T) {
	tr := NewDurationTracker()
	bigStep(tr)
	t0 := time.Now()
	tr.Eval("k", t0, true, time.Minute, 30*time.Minute, false)
	r := tr.Eval("k", t0.Add(time.Minute), true, time.Minute, 30*time.Minute, false)
	if !r.NewAlert {
		t.Fatal("跃迁应告警")
	}
	// 持续中,29m 内不重复
	r = tr.Eval("k", t0.Add(30*time.Minute), true, time.Minute, 30*time.Minute, false)
	if !r.Firing || r.NewAlert {
		t.Fatalf("去重窗口内不应续报: %+v", r)
	}
	// 超过 30m 续报
	r = tr.Eval("k", t0.Add(31*time.Minute), true, time.Minute, 30*time.Minute, false)
	if !r.NewAlert {
		t.Fatal("超过去重窗口应续报")
	}
}

// 重启恢复:首次评估且历史证明过去 dur 一直成立 → 立即 firing(无 30m 盲区)。
func TestDuration_HistoricalHold(t *testing.T) {
	tr := NewDurationTracker()
	r := tr.Eval("k", time.Now(), true, 30*time.Minute, 30*time.Minute, true)
	if !r.Firing || !r.NewAlert {
		t.Fatalf("历史持续成立应立即跃迁: %+v", r)
	}
}

// ---------- F2-7 时钟漂移演练(注入式,默认 MaxStep=2min) ----------

// 时钟向前跳变(+5min/次):无防护的墙钟差模型 2 次跳变即累计 10m 误报;
// 单调累计制每次 cap 2m,必须真实累计满 10m 才 firing。
func TestClockDrift_ForwardJump_NoEarlyFire(t *testing.T) {
	tr := NewDurationTracker() // 默认 MaxStep=2m
	t0 := time.Now()
	dur := 10 * time.Minute
	tr.Eval("k", t0, true, dur, 30*time.Minute, false)
	// 连续 4 次 +5min 跳变:墙钟已过 20m,但累计只有 8m → 不允许 firing
	for i := 1; i <= 4; i++ {
		r := tr.Eval("k", t0.Add(time.Duration(i)*5*time.Minute), true, dur, 30*time.Minute, false)
		if r.Firing {
			t.Fatalf("第 %d 次跳变后 firing,时钟漂移骗过了阈值(累计应 %dm)", i, i*2)
		}
	}
	// 第 5 次:累计 10m 达到阈值 → 必须 firing(该报的不能漏)
	r := tr.Eval("k", t0.Add(25*time.Minute), true, dur, 30*time.Minute, false)
	if !r.Firing || !r.NewAlert {
		t.Fatalf("累计满 10m 应跃迁: %+v", r)
	}
}

// 时钟回退(-5min):累计不清零、不倒退;恢复正向后按真实采样节奏继续。
func TestClockDrift_BackwardJump_NoReset(t *testing.T) {
	tr := NewDurationTracker()
	t0 := time.Now()
	dur := 10 * time.Minute
	// 正常每分钟采样 8 次:累计 7m(首次 delta=0)
	for i := 0; i < 8; i++ {
		tr.Eval("k", t0.Add(time.Duration(i)*time.Minute), true, dur, 30*time.Minute, false)
	}
	// 时钟回退 5min(t0+8m → t0+3m):增量 0,累计保持 7m,不 firing 不清零
	r := tr.Eval("k", t0.Add(3*time.Minute), true, dur, 30*time.Minute, false)
	if r.Firing {
		t.Fatal("回退时刻不应 firing(累计 7m < 10m)")
	}
	// 正向恢复:再累计 3m 满 10m → firing
	for _, at := range []time.Duration{4, 5} {
		r = tr.Eval("k", t0.Add(at*time.Minute), true, dur, 30*time.Minute, false)
		if r.Firing {
			t.Fatalf("累计未满不应 firing(%dm 处)", at)
		}
	}
	r = tr.Eval("k", t0.Add(6*time.Minute), true, dur, 30*time.Minute, false)
	if !r.Firing || !r.NewAlert {
		t.Fatalf("累计满 10m 应跃迁: %+v", r)
	}
}

// 告警去重防跳变:firing 后时钟猛跳 +31min,续报仍须按累计 30m 才发(不被一次跳变骗出续报)。
func TestClockDrift_DedupNotFooled(t *testing.T) {
	tr := NewDurationTracker()
	t0 := time.Now()
	tr.Eval("k", t0, true, time.Minute, 30*time.Minute, false)
	if r := tr.Eval("k", t0.Add(time.Minute), true, time.Minute, 30*time.Minute, false); !r.NewAlert {
		t.Fatal("跃迁应告警")
	}
	// 时钟单次跳前 31min:无防护会立即续报;累计制只 +2m,不续报
	r := tr.Eval("k", t0.Add(32*time.Minute), true, time.Minute, 30*time.Minute, false)
	if !r.Firing {
		t.Fatal("跳变不应打掉 firing 状态")
	}
	if r.NewAlert {
		t.Fatal("一次大跳变骗出了续报(累计仅 2m < 30m)")
	}
	// 再正常累计:跳变已计 2m,还需 28m;27 次 ×1m 后到 29m 不续报,第 28 次满 30m 续报
	for i := 0; i < 27; i++ {
		r = tr.Eval("k", t0.Add(33*time.Minute+time.Duration(i)*time.Minute), true, time.Minute, 30*time.Minute, false)
		if r.NewAlert {
			t.Fatalf("累计未满 30m 不应续报(第 %d 次, 已累计 %dm)", i, 2+i+1)
		}
	}
	r = tr.Eval("k", t0.Add(60*time.Minute), true, time.Minute, 30*time.Minute, false)
	if !r.NewAlert {
		t.Fatal("累计满 30m 应续报")
	}
}
