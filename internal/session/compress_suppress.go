package session

import (
	"sync"
	"time"
)

// compressSuppression records the latest compression failure for a session.
// 移植自 Grok auto_compact_suppressed：确定性失败 sticky 到模型切换，
// 瞬态失败按 minGap 退避（避免注定/无意义的重试每 turn 重复打 LLM）。
type compressSuppression struct {
	kind          compressFailureKind
	providerModel string // 压缩模型 "provider/model"
	at            time.Time
}

// compressSuppressManager tracks per-session compression failure suppression.
// 进程内内存态：重启后重新尝试一次再抑制，代价可接受（与 Grok shell 同语义）。
type compressSuppressManager struct {
	m sync.Map // map[sessionID]compressSuppression
}

func newCompressSuppressManager() *compressSuppressManager {
	return &compressSuppressManager{}
}

// check reports whether compression should be suppressed for the session.
// deterministic: suppressed while the compress model is unchanged.
// transient: suppressed within minGap of the last failure.
func (s *compressSuppressManager) check(sessionID, providerModel string, minGap time.Duration, now time.Time) (bool, string) {
	v, ok := s.m.Load(sessionID)
	if !ok {
		return false, ""
	}
	sup := v.(compressSuppression)
	switch sup.kind {
	case compressFailureDeterministic:
		if sup.providerModel == providerModel {
			return true, "deterministic_failure"
		}
		return false, "" // 模型已切换，抑制解除
	case compressFailureTransient:
		if minGap > 0 && now.Sub(sup.at) < minGap {
			return true, "transient_backoff"
		}
		return false, ""
	default:
		return false, ""
	}
}

func (s *compressSuppressManager) record(sessionID string, kind compressFailureKind, providerModel string, now time.Time) {
	if kind == compressFailureNone || sessionID == "" {
		return
	}
	s.m.Store(sessionID, compressSuppression{kind: kind, providerModel: providerModel, at: now})
}

func (s *compressSuppressManager) clear(sessionID string) {
	s.m.Delete(sessionID)
}
