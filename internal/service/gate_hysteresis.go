package service

import (
	"sync"
	"time"
)

// gateHysteresisStore 预规划门控的会话级滞回（P2-⑤，session-eval-20260829-r2
// R4-Q9 根修）。
//
// 背景：门控逐轮独立打分，话题漂移序列中档位 simple(0.15)→moderate(0.3,
// force)→simple(0.1)→… 数轮内反复横跳，moderate 轮强制规划后又立即回落。
// 滞回规则：会话首轮直采定档；续轮 raw 与生效档一致则归零计数；反向需
// **连续 2 轮同向越阈**才切档。单轮尖峰被压住，真实持续升级仅迟滞一轮。
//
// 平滑对象是最终 ForcePlanning 布尔（词法豁免已在 Evaluate 内结算），
// level/score 原文透传（仅日志/提示文案，不影响路由）。
//
// 存储为进程内热缓存（与 pendingClarifications 同构）：重启丢失滞回历史
// 仅意味着首轮直采，代价可接受；不持久化避免 DB 往返进关键路径。
const (
	// gateHysteresisSwitchThreshold 切档所需连续同向越阈轮数。
	gateHysteresisSwitchThreshold = 2
	// gateHysteresisMaxSessions 热缓存容量；超容淘汰最久未活跃会话。
	gateHysteresisMaxSessions = 4096
)

type gateHysteresisEntry struct {
	gearSet      bool      // 首轮已直采定档
	gear         bool      // 当前生效档（true=强制规划）
	pendingCount int       // 连续反向越阈计数
	lastSeen     time.Time // 最近一轮时间（LRU 淘汰依据）
}

type gateHysteresisStore struct {
	mu       sync.Mutex
	sessions map[string]*gateHysteresisEntry
}

func newGateHysteresisStore() *gateHysteresisStore {
	return &gateHysteresisStore{sessions: make(map[string]*gateHysteresisEntry)}
}

// Apply 返回滞回后的生效档位。damped=true 表示本轮 raw 反向越阈但未满连续
// 阈值，被滞回压住（维持原档）。
func (s *gateHysteresisStore) Apply(sessionID string, raw bool) (gear bool, damped bool) {
	if s == nil || sessionID == "" {
		return raw, false
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.sessions[sessionID]
	if !ok {
		s.evictLocked()
		e = &gateHysteresisEntry{gearSet: true, gear: raw, lastSeen: now}
		s.sessions[sessionID] = e
		return raw, false
	}
	e.lastSeen = now
	if !e.gearSet {
		e.gearSet = true
		e.gear = raw
		return raw, false
	}
	if raw == e.gear {
		e.pendingCount = 0
		return e.gear, false
	}
	e.pendingCount++
	if e.pendingCount >= gateHysteresisSwitchThreshold {
		e.gear = raw
		e.pendingCount = 0
		return e.gear, false
	}
	return e.gear, true
}

// evictLocked 超容时淘汰最久未活跃会话（容量 4096，线性扫描代价可忽略）。
func (s *gateHysteresisStore) evictLocked() {
	if len(s.sessions) < gateHysteresisMaxSessions {
		return
	}
	var oldestID string
	var oldest time.Time
	first := true
	for id, e := range s.sessions {
		if first || e.lastSeen.Before(oldest) {
			first = false
			oldestID, oldest = id, e.lastSeen
		}
	}
	if oldestID != "" {
		delete(s.sessions, oldestID)
	}
}
