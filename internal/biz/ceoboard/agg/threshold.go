// 阈值"持续时间"语义。
// 声明形如:"=0 持续30m"、"<1 持续30m 且探针活"、">20"(即时)。
// 语义:
//   - 即时阈值:当前值命中即生效;
//   - 持续阈值:条件须连续成立 ≥ 持续时长才跃迁;期间任意一次不成立(抖动中断)即重新计时;
//   - 恢复:条件不再成立即回落绿色,跃迁可逆;
//   - 告警去重:跃迁触发一次告警;持续告警期间同一指标 30m 内不重复刷屏;
//   - 重启恢复:首次评估时若历史数据证明过去 dur 内条件一直成立(historicalHold),直接以已持续 dur 计,
//     避免重启后 30m 盲区。
package agg

import (
	"regexp"
	"strconv"
	"sync"
	"time"
)

// ThreshExpr 是解析后的阈值表达式。
type ThreshExpr struct {
	Op        string        // > >= < <= = ;空 = 表达式不可解析
	Limit     float64       //
	Dur       time.Duration // 持续时长;0 = 即时阈值
	NeedProbe bool          // 后缀 "且探针活":条件须同时满足探针存活
	Raw       string        //
}

var (
	thHeadRe = regexp.MustCompile(`^\s*(>=|<=|>|<|=)\s*(\d+(?:\.\d+)?)\s*(.*)$`)
	thDurRe  = regexp.MustCompile(`持续\s*(\d+)\s*(m|h|s)`)
)

// ParseThresh 解析阈值字符串。不可解析返回 Op=""。
func ParseThresh(s string) ThreshExpr {
	e := ThreshExpr{Raw: s}
	m := thHeadRe.FindStringSubmatch(s)
	if len(m) != 4 {
		return e
	}
	e.Op = m[1]
	e.Limit, _ = strconv.ParseFloat(m[2], 64)
	rest := m[3]
	if dm := thDurRe.FindStringSubmatch(rest); len(dm) == 3 {
		n, _ := strconv.Atoi(dm[1])
		switch dm[2] {
		case "s":
			e.Dur = time.Duration(n) * time.Second
		case "m":
			e.Dur = time.Duration(n) * time.Minute
		case "h":
			e.Dur = time.Duration(n) * time.Hour
		}
	}
	if regexp.MustCompile(`且探针活`).MatchString(rest) {
		e.NeedProbe = true
	}
	return e
}

// Match 判定当前值是否命中条件(不含持续/探针部分)。
func (e ThreshExpr) Match(v float64) bool {
	switch e.Op {
	case ">":
		return v > e.Limit
	case ">=":
		return v >= e.Limit
	case "<":
		return v < e.Limit
	case "<=":
		return v <= e.Limit
	case "=":
		return v == e.Limit
	}
	return false
}

// durState 是单个 (指标×级别) 的持续判定状态。
// 时钟漂移防护:持续时长采用单调累计制,不用墙钟差:
//   - 每次 Eval 只累计 clamp(now-lastTick, 0, MaxStep);时钟回退 → 增量 0(不清零不前进);
//   - 时钟跳前(如 +5min)→ 增量被 cap 到 MaxStep,不会瞬间满足 30m 阈值;
//   - 告警去重同样用累计制(firing 期间累计 ≥ dedup 才续报),跳变骗不出提前续报。
type durState struct {
	accrued    time.Duration // 条件连续成立的累计时长(单调)
	lastTick   time.Time     // 上次 Eval 的 now(零值=未采样)
	firing     bool          // 是否已跃迁(持续满足)
	sinceAlert time.Duration // firing 期间距上次告警的累计时长
	since      time.Time     // 展示用:连续成立起点(墙钟,仅展示)
}

// DurationTracker 跟踪全部指标的持续阈值状态(Aggregator 持有,跨 Build 存活)。
type DurationTracker struct {
	mu      sync.Mutex
	states  map[string]*durState
	MaxStep time.Duration // 单次 Eval 允许累计的最大步长(时钟跳变防护);0 = 默认 2min
}

// NewDurationTracker 构造。
func NewDurationTracker() *DurationTracker {
	return &DurationTracker{states: map[string]*durState{}, MaxStep: 2 * time.Minute}
}

// DurationResult 是一次持续阈值评估结果。
type DurationResult struct {
	Firing    bool // 当前是否处于"持续命中"状态(决定健康度)
	NewAlert  bool // 本次是否应产生告警(跃迁 或 去重窗口后的续报)
	Recovered bool // 本次是否发生  firing→恢复 跃迁
	Since     time.Time
}

// Eval 评估一次持续阈值。
//
//	cond: 当前采样条件是否成立(调用方算好值与探针);
//	historicalHold: 历史数据是否证明过去 dur 内条件一直成立(首次评估/重启恢复用);
//	dedup: 告警去重窗口(30m)。
func (t *DurationTracker) Eval(key string, now time.Time, cond bool, dur, dedup time.Duration, historicalHold bool) DurationResult {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.states[key]
	if !ok {
		st = &durState{}
		t.states[key] = st
	}
	maxStep := t.MaxStep
	if maxStep <= 0 {
		maxStep = 2 * time.Minute
	}
	res := DurationResult{}

	// 单调增量:回退→0;跳前→cap
	var delta time.Duration
	if !st.lastTick.IsZero() {
		delta = now.Sub(st.lastTick)
		if delta < 0 {
			delta = 0
		} else if delta > maxStep {
			delta = maxStep
		}
	}
	st.lastTick = now

	if !cond {
		// 抖动中断/恢复:清零累计;若之前在 firing,记录恢复跃迁
		if st.firing {
			res.Recovered = true
		}
		st.firing = false
		st.accrued = 0
		st.sinceAlert = 0
		st.since = time.Time{}
		res.Firing = false
		return res
	}

	if st.since.IsZero() {
		st.since = now
	}
	res.Since = st.since
	if st.accrued == 0 && historicalHold {
		// 历史已证明持续成立(如重启后):视为已持续 dur,立即进入跃迁判定
		st.accrued = dur
	}
	st.accrued += delta
	if st.accrued < dur {
		res.Firing = st.firing // 未满足时长:维持原状态(去抖前夜)
		return res
	}
	// 满足时长
	if !st.firing {
		st.firing = true
		st.sinceAlert = 0
		res.Firing = true
		res.NewAlert = true // 跃迁告警
		return res
	}
	res.Firing = true
	// 持续告警去重:firing 期间累计 ≥ dedup 才续报
	st.sinceAlert += delta
	if st.sinceAlert >= dedup {
		st.sinceAlert = 0
		res.NewAlert = true
	}
	return res
}

// Reset 清除某 key 状态(测试用)。
func (t *DurationTracker) Reset(key string) {
	t.mu.Lock()
	delete(t.states, key)
	t.mu.Unlock()
}
