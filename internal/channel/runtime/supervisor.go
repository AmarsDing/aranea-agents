package runtime

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	arametrics "aranea-agents/internal/metrics"
)

const (
	runtimeReconnectInitial = time.Second
	runtimeReconnectMax     = 5 * time.Minute
)

func (m *Manager) runSupervised(
	parentCtx context.Context,
	ch biz.Channel,
	fp string,
	starter Starter,
	platform, mode string,
) {
	defer m.remove(ch.ID)
	arametrics.ChannelRuntimeConnected.WithLabelValues(platform, mode).Inc()
	defer arametrics.ChannelRuntimeConnected.WithLabelValues(platform, mode).Dec()

	backoff := runtimeReconnectInitial
	for {
		if parentCtx.Err() != nil {
			return
		}
		if !m.fingerprintMatches(ch.ID, fp) {
			return
		}
		creds, err := m.channels.ListCredentialsRaw(parentCtx, ch.ID)
		if err != nil {
			return
		}
		_ = starter(parentCtx, ch, creds, m.credLookup, m.handler)
		if parentCtx.Err() != nil {
			return
		}
		if !m.fingerprintMatches(ch.ID, fp) {
			return
		}
		arametrics.ChannelRuntimeReconnectTotal.WithLabelValues(platform, mode, "disconnect").Inc()
		arametrics.ChannelRuntimeReconnectTotal.WithLabelValues(platform, mode, "attempt").Inc()

		timer := time.NewTimer(backoff)
		select {
		case <-parentCtx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < runtimeReconnectMax {
			backoff *= 2
			if backoff > runtimeReconnectMax {
				backoff = runtimeReconnectMax
			}
		}
	}
}

func (m *Manager) fingerprintMatches(id, fp string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	inst, ok := m.running[id]
	return ok && inst.fingerprint == fp
}
