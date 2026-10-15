package voice

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"time"
)

func (s *Session) resetIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(idleReclaimTimeout, s.reclaimIdleASR)
}

func (s *Session) reclaimIdleASR() {
	s.mu.Lock()
	asr := s.asr
	s.asr = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
		s.lg.Info("voice asr: idle upstream reclaimed", loggateway.StepID("voice.asr.idle_reclaim"))
	}
}

func (s *Session) teardown() {
	s.mu.Lock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}

	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
		s.sleepTimer = nil
	}
	s.stopSpeculationLocked()
	asr := s.asr
	s.asr = nil
	sch := s.scheduler
	s.scheduler = nil
	s.chunker = nil
	s.schedGen++
	ttsProv := s.ttsProvider
	s.ttsProvider = nil
	s.ttsCfg = biz.TTSSessionConfig{}
	s.utterBuf = nil
	s.utterOverflow = false
	unsub := s.unsub
	s.unsub = nil
	s.mu.Unlock()
	if asr != nil {
		_ = asr.Close()
	}
	if sch != nil {
		sch.Cancel()
	}

	if pw, ok := ttsProv.(ttsConnPrewarmer); ok {
		pw.ReleaseWarmTTSConn()
	}

	if reg := s.deps.Delegation; reg != nil {
		reg.ClearVoiceSession(s.sessionID)
	}
	if unsub != nil {
		unsub()
	}
}
