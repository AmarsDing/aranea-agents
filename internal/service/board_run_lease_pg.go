package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// board_run_lease_pg.go —— P0-6 阶段 1a（90-multi-replica-board-lease）：
// biz.BoardRunLeasePort 的 PG 行租约实现 + capability flag（
// ARANEA_BOARD_RUN_LEASE）。
//
// 语义与边界：
//   - owner = "<host>:<pid>:<boot-nanos>" 进程实例身份；TTL 默认 30s，
//     TryClaim 成功后 per-board 心跳 goroutine（ttl/2 间隔）续约。
//   - 心跳续约失败（renewed=false = 行被领养/删除/过期）→ 立即调用
//     lease.Cancel 杀掉本进程僵尸 run（单写者底线的兜底）；
//     DB 错误按重试计，连续 3 次失败同样 cancel（防 DB 抖动期间僵尸续跑）。
//   - TryClaim 出错一律 fail-closed：claimed=false（多副本下拿不到租约
//     绝不能动；单副本 flag=pg 时 DB 挂了也宁可不派发，等 DB 恢复/租约过期）。
//   - HasActiveForSession 出错 fail-closed 返回 true（波次中点门控宁误拦
//     synthesis 不放行重复总结；端口语义文档化于 biz.BoardRunLeasePort）。
//   - Release 幂等、owner-scoped；停心跳先于删行，释放自身失败仅 Warn
//     （残留行到期收养兜底，不炸派发链路）。
//
// flag 档位（ResolveBoardRunLeaseMode）：""（缺省）= 进程内
// boardRunRegistry 字节级旧行为（端口不接线）；mem = InMem 端口试跑档
// （端口语义等价性灰度，无跨进程语义）；pg = 本实现。
const (
	EnvBoardRunLeaseMode = "ARANEA_BOARD_RUN_LEASE"

	BoardRunLeaseModeOff = ""
	BoardRunLeaseModeMem = "mem"
	BoardRunLeaseModePG  = "pg"

	boardRunLeaseDefaultTTL = 30 * time.Second
	boardRunLeaseMaxRenewFail = 3
)

// ResolveBoardRunLeaseMode 纯函数解析 flag（无副作用，nil getenv 回落
// os.Getenv）；非法值一律按缺省 off（fail-safe：不认得的档位=旧行为）。
func ResolveBoardRunLeaseMode(getenv func(string) string) string {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv(EnvBoardRunLeaseMode))) {
	case BoardRunLeaseModeMem:
		return BoardRunLeaseModeMem
	case BoardRunLeaseModePG:
		return BoardRunLeaseModePG
	default:
		return BoardRunLeaseModeOff
	}
}

// hbEntry 心跳句柄：id 区分先后代——同 board 重入重建时旧 goroutine 退出
// 不得误删新句柄（对抗评审 #1：退出经验证身份后自清，杜绝句柄泄漏/误删）。
type hbEntry struct {
	id   int64
	stop context.CancelFunc
}

// PGBoardRunLease 是 biz.BoardRunLeasePort 的 PG 行租约实现（阶段 1a）。
type PGBoardRunLease struct {
	repo  biz.BoardRunLeaseRepo
	owner string
	ttl   time.Duration
	lg    loggateway.Logger

	mu    sync.Mutex
	hb    map[string]hbEntry // boardID → 心跳 goroutine 句柄
	hbSeq int64              // 句柄代际计数（mu 保护）
}

var _ biz.BoardRunLeasePort = (*PGBoardRunLease)(nil)

// PGBoardRunLeaseOptions 装配参数：Owner 缺省 "host:pid:boot-nanos"，
// TTL 缺省 30s。
type PGBoardRunLeaseOptions struct {
	Owner  string
	TTL    time.Duration
	Logger loggateway.Logger
}

// NewPGBoardRunLease repo 为 nil 时返回 nil（adapter 侧按 flag=off 处理；
// 装配层见 cmd/admin provideBoardRunLeasePort）。
func NewPGBoardRunLease(repo biz.BoardRunLeaseRepo, opts PGBoardRunLeaseOptions) *PGBoardRunLease {
	if repo == nil {
		return nil
	}
	owner := strings.TrimSpace(opts.Owner)
	if owner == "" {
		host, _ := os.Hostname()
		owner = fmt.Sprintf("%s:%d:%d", host, os.Getpid(), time.Now().UnixNano())
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = boardRunLeaseDefaultTTL
	}
	lg := opts.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &PGBoardRunLease{repo: repo, owner: owner, ttl: ttl, lg: lg, hb: make(map[string]hbEntry)}
}

// OwnerID 暴露实例身份（healthz/自检观测位）。
func (l *PGBoardRunLease) OwnerID() string {
	if l == nil {
		return ""
	}
	return l.owner
}

// TryClaim PG 行认领（仅过期行可收养）；成功后启动 per-board 心跳。
// 任何错误 fail-closed → claimed=false。
func (l *PGBoardRunLease) TryClaim(ctx context.Context, lease biz.BoardRunLease) (claimed bool) {
	if l == nil || l.repo == nil || lease.BoardID == "" {
		return false
	}
	claimed, err := l.repo.TryClaimBoardRunLease(ctx, lease, l.owner, l.ttl)
	if err != nil {
		l.lg.Warn("board run lease 认领失败（fail-closed，不派发）",
			loggateway.Str("plan_board_id", lease.BoardID),
			loggateway.Err(err))
		return false
	}
	if !claimed {
		return false
	}
	l.startHeartbeat(lease.BoardID, lease.Cancel)
	return true
}

// boardRunLeaseRenewTimeout 单次续约边界：min(10s, ttl/2)，下限 1s。
// 对抗评审 #3：DB 驱动卡死时心跳/主动续约不可无限挂起——卡死期间行
// 静默过期被领养，renewed=false 杀僵尸的兜底必须能及时走到。
func boardRunLeaseRenewTimeout(ttl time.Duration) time.Duration {
	d := ttl / 2
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	if d < 1*time.Second {
		d = 1 * time.Second
	}
	return d
}

// Renew 主动续约（端口语义）；心跳连续失败计数不受主动续约影响。
func (l *PGBoardRunLease) Renew(ctx context.Context, boardID string) bool {
	if l == nil || l.repo == nil || boardID == "" {
		return false
	}
	renewCtx, cancel := context.WithTimeout(ctx, boardRunLeaseRenewTimeout(l.ttl))
	defer cancel()
	renewed, err := l.repo.RenewBoardRunLease(renewCtx, boardID, l.owner, l.ttl)
	if err != nil {
		l.lg.Warn("board run lease 续约失败",
			loggateway.Str("plan_board_id", boardID),
			loggateway.Err(err))
		return false
	}
	return renewed
}

// Release 停心跳 + owner-scoped 删行；幂等（删除失败仅 Warn——残留行
// 到期收养兜底）。
func (l *PGBoardRunLease) Release(boardID string) {
	if l == nil || l.repo == nil || boardID == "" {
		return
	}
	l.stopHeartbeat(boardID)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := l.repo.ReleaseBoardRunLease(ctx, boardID, l.owner); err != nil {
		l.lg.Warn("board run lease 释放失败（残留行到期收养兜底）",
			loggateway.Str("plan_board_id", boardID),
			loggateway.Err(err))
	}
}

// HasActiveForSession 跨副本波次中点门控。DB 错误 fail-closed 返回 true
// （宁误拦 synthesis 不放行重复总结——P0-6 单写者立场）。
func (l *PGBoardRunLease) HasActiveForSession(sessionID string) bool {
	if l == nil || l.repo == nil || sessionID == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	active, err := l.repo.HasActiveBoardRunForSession(ctx, sessionID, time.Now())
	if err != nil {
		l.lg.Warn("board run lease 活跃查询失败（fail-closed 门控拦截）",
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return true
	}
	return active
}

// startHeartbeat per-board 心跳：ttl/2 周期续约；renewed=false（行被领/
// 删/过期）或连续 boardRunLeaseMaxRenewFail 次 DB 错误 → cancel 本进程
// 僵尸 run 并退出。cancel 可为 nil（无 run 可杀时仅停心跳）。
// goroutine 退出时按代际身份自清句柄（对抗评审 #1），单次续约带边界
// 子 ctx（对抗评审 #3，见 boardRunLeaseRenewTimeout）。
func (l *PGBoardRunLease) startHeartbeat(boardID string, cancel context.CancelFunc) {
	ctx, stop := context.WithCancel(context.Background())
	l.mu.Lock()
	l.hbSeq++
	myID := l.hbSeq
	if prev, ok := l.hb[boardID]; ok {
		prev.stop() // 防重复心跳（极端重入）
	}
	l.hb[boardID] = hbEntry{id: myID, stop: stop}
	l.mu.Unlock()
	safego.Go(ctx, "board_run_lease.hb."+boardID, func() {
		defer func() {
			// 自清句柄：仅当句柄仍是本代——重入换代后句柄属新 goroutine，
			// 不得误删（对抗评审 #1：杀僵尸路径原裸退导致 hb 泄漏）。
			l.mu.Lock()
			if cur, ok := l.hb[boardID]; ok && cur.id == myID {
				delete(l.hb, boardID)
			}
			l.mu.Unlock()
		}()
		defer stop()
		ticker := time.NewTicker(l.ttl / 2)
		defer ticker.Stop()
		fails := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			renewCtx, renewCancel := context.WithTimeout(ctx, boardRunLeaseRenewTimeout(l.ttl))
			renewed, err := l.repo.RenewBoardRunLease(renewCtx, boardID, l.owner, l.ttl)
			renewCancel()
			switch {
			case err != nil:
				fails++
				l.lg.Warn("board run lease 心跳续约 DB 错误",
					loggateway.Str("plan_board_id", boardID),
					loggateway.Int("consecutive_failures", fails),
					loggateway.Err(err))
				if fails < boardRunLeaseMaxRenewFail {
					continue
				}
				l.lg.Warn("board run lease 心跳连续失败，杀掉本地僵尸 run",
					loggateway.Str("plan_board_id", boardID))
			case !renewed:
				l.lg.Warn("board run lease 已丢失（行被领养/过期），杀掉本地僵尸 run",
					loggateway.Str("plan_board_id", boardID))
			default:
				fails = 0
				continue
			}
			if cancel != nil {
				cancel()
			}
			return
		}
	})
}

func (l *PGBoardRunLease) stopHeartbeat(boardID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if entry, ok := l.hb[boardID]; ok {
		delete(l.hb, boardID)
		entry.stop()
	}
}

// ActiveHeartbeats 心跳 goroutine 计数（测试/自检观测位，非端口成员）。
func (l *PGBoardRunLease) ActiveHeartbeats() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.hb)
}
