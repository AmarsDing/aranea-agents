package runtime

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
)

// TurnLane classifies turn workloads for global concurrency scheduling (CH-BOR-13).
type TurnLane string

const (
	TurnLaneMain    TurnLane = "main"
	TurnLaneCron    TurnLane = "cron"
	TurnLaneTeam    TurnLane = "team"
	TurnLaneDurable TurnLane = "durable"
)

// LaneLimits configures max in-flight turns per lane. Zero or negative means unlimited.
type LaneLimits struct {
	Main    int
	Cron    int
	Team    int
	Durable int
}

// DefaultLaneLimits keeps interactive paths unlimited while bounding background lanes.
func DefaultLaneLimits() LaneLimits {
	return LaneLimits{
		Main:    0,
		Cron:    2,
		Team:    2,
		Durable: 4,
	}
}

// TurnLaneFromEntry maps turn entry metadata to a scheduler lane.
func TurnLaneFromEntry(entry biz.TurnEntryPoint, ownerType string) TurnLane {
	switch entry {
	case biz.EntryPointCron:
		return TurnLaneCron
	case biz.EntryPointDurable:
		return TurnLaneDurable
	default:
		if strings.EqualFold(strings.TrimSpace(ownerType), "team") {
			return TurnLaneTeam
		}
		return TurnLaneMain
	}
}

// LaneScheduler limits concurrent turns per lane so Cron/Team cannot starve Channel/Web.
type LaneScheduler struct {
	limits LaneLimits
	mu     sync.Mutex
	active map[TurnLane]int
	wait   map[TurnLane][]*laneWaiter
}

// laneWaiter 排队等位单元。dropped 由取消方置位（与 send 同锁），release
// 扫队列时跳过，避免向已取消 waiter 投递信号造成 lost wakeup。
type laneWaiter struct {
	ch      chan struct{}
	dropped bool
}

// DefaultLaneScheduler is the process-wide lane gate used by ChatOrchestrator.
var DefaultLaneScheduler = NewLaneScheduler(DefaultLaneLimits())

// NewLaneScheduler constructs a lane scheduler with the given limits.
func NewLaneScheduler(limits LaneLimits) *LaneScheduler {
	return &LaneScheduler{
		limits: limits,
		active: make(map[TurnLane]int),
		wait:   make(map[TurnLane][]*laneWaiter),
	}
}

// Acquire blocks until the lane has capacity or ctx is cancelled.
//
// P1 FR-6 (89-session-concurrency-hardening / M-1)：返回值从单 release 改为
// (release, ok)——ctx 取消时返回 (no-op, false)，调用方必须检查并终止
// turn，不得放行无槽位执行。nil 调度器与无上限车道（limit<=0）恒 ok=true。
func (s *LaneScheduler) Acquire(ctx context.Context, lane TurnLane) (func(), bool) {
	if s == nil {
		return func() {}, true
	}
	limit := s.limitFor(lane)
	if limit <= 0 {
		return func() {}, true
	}
	for {
		s.mu.Lock()
		if s.active[lane] < limit {
			s.active[lane]++
			s.mu.Unlock()
			return func() { s.release(lane) }, true
		}
		w := &laneWaiter{ch: make(chan struct{}, 1)}
		s.wait[lane] = append(s.wait[lane], w)
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			// 交接竞态：release 可能已在 select 汇合前把信号送入 w.ch——
			// 丢弃会丢唤醒（后位 waiter 空转卡死）。drain 检查并转交。
			if s.dropWaiter(lane, w) {
				s.signalNext(lane)
			}
			return func() {}, false
		case <-w.ch:
		}
	}
}

func (s *LaneScheduler) limitFor(lane TurnLane) int {
	switch lane {
	case TurnLaneCron:
		return s.limits.Cron
	case TurnLaneTeam:
		return s.limits.Team
	case TurnLaneDurable:
		return s.limits.Durable
	default:
		return s.limits.Main
	}
}

func (s *LaneScheduler) release(lane TurnLane) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active[lane] > 0 {
		s.active[lane]--
	}
	s.signalNextLocked(lane)
}

// signalNextLocked 向队首存活 waiter 投递唤醒。已 dropped 或信号已投递
// （该类 waiter 即将苏醒自行重查）的直接跳过。槽位计数不动——被唤醒者
// 回到 Acquire 循环顶部自行 active++，计数与位次在锁内严格配对。
func (s *LaneScheduler) signalNextLocked(lane TurnLane) {
	q := s.wait[lane]
	for len(q) > 0 {
		w := q[0]
		s.wait[lane] = q[1:]
		q = q[1:]
		if w.dropped {
			continue
		}
		select {
		case w.ch <- struct{}{}:
			return
		default:
			continue
		}
	}
}

func (s *LaneScheduler) signalNext(lane TurnLane) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signalNextLocked(lane)
}

// dropWaiter 将 waiter 从等位队列摘除并标记 dropped；返回该 waiter 是否
// 已被 release 交接过信号（已送达但 select 里输给 ctx 分支）——true 时
// 调用方须代其转交唤醒，避免槽位信号空转。
func (s *LaneScheduler) dropWaiter(lane TurnLane, target *laneWaiter) bool {
	s.mu.Lock()
	target.dropped = true
	q := s.wait[lane]
	out := q[:0]
	for _, w := range q {
		if w != target {
			out = append(out, w)
		}
	}
	s.wait[lane] = out
	s.mu.Unlock()
	// 已被 release 弹出的 waiter：信号已投递进缓冲通道，drain 判定转交义务。
	select {
	case <-target.ch:
		return true
	default:
		return false
	}
}

// InFlight returns active turn count for a lane (testing/diagnostics).
func (s *LaneScheduler) InFlight(lane TurnLane) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[lane]
}

// AcquireTurnLane is a convenience wrapper around DefaultLaneScheduler.
// P1 FR-6：取消即 (no-op, false)——调用方据此终止 turn。
func AcquireTurnLane(ctx context.Context, input biz.TurnInput, ownerType string) (func(), bool) {
	return DefaultLaneScheduler.Acquire(ctx, TurnLaneFromEntry(input.EntryConfig.EntryPoint, ownerType))
}
