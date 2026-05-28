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
	wait   map[TurnLane][]chan struct{}
}

// DefaultLaneScheduler is the process-wide lane gate used by ChatOrchestrator.
var DefaultLaneScheduler = NewLaneScheduler(DefaultLaneLimits())

// NewLaneScheduler constructs a lane scheduler with the given limits.
func NewLaneScheduler(limits LaneLimits) *LaneScheduler {
	return &LaneScheduler{
		limits: limits,
		active: make(map[TurnLane]int),
		wait:   make(map[TurnLane][]chan struct{}),
	}
}

// Acquire blocks until the lane has capacity or ctx is cancelled. Returns a release func.
func (s *LaneScheduler) Acquire(ctx context.Context, lane TurnLane) func() {
	if s == nil {
		return func() {}
	}
	limit := s.limitFor(lane)
	if limit <= 0 {
		return func() {}
	}
	for {
		s.mu.Lock()
		if s.active[lane] < limit {
			s.active[lane]++
			s.mu.Unlock()
			return func() { s.release(lane) }
		}
		waitCh := make(chan struct{}, 1)
		s.wait[lane] = append(s.wait[lane], waitCh)
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			s.dropWaiter(lane, waitCh)
			return func() {}
		case <-waitCh:
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
	q := s.wait[lane]
	for len(q) > 0 {
		ch := q[0]
		select {
		case ch <- struct{}{}:
			s.wait[lane] = q[1:]
			return
		default:
			q = q[1:]
		}
	}
	s.wait[lane] = q
}

func (s *LaneScheduler) dropWaiter(lane TurnLane, target chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := s.wait[lane]
	out := q[:0]
	for _, ch := range q {
		if ch != target {
			out = append(out, ch)
		}
	}
	s.wait[lane] = out
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
func AcquireTurnLane(ctx context.Context, input biz.TurnInput, ownerType string) func() {
	return DefaultLaneScheduler.Acquire(ctx, TurnLaneFromEntry(input.EntryConfig.EntryPoint, ownerType))
}
