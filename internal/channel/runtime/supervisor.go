package runtime

import (
	"context"
	"encoding/json"
	"strings"
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

	backoff := wsReconnectInterval(ch.ConfigJSON)
	if backoff <= 0 {
		backoff = runtimeReconnectInitial
	}
	for {
		if parentCtx.Err() != nil {
			return
		}
		if !m.fingerprintMatches(ch.ID, fp) {
			return
		}
		setChannelConnection(ch.ID, true)
		creds, err := m.channels.ListCredentialsRaw(parentCtx, ch.ID)
		if err != nil {
			setChannelConnection(ch.ID, false)
			return
		}
		_ = starter(parentCtx, ch, creds, m.credLookup, m.handler)
		setChannelConnection(ch.ID, false)
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

func wsReconnectInterval(configJSON string) time.Duration {
	var cfg struct {
		Config struct {
			WSReconnectIntervalSec int `json:"ws_reconnect_interval_sec"`
		} `json:"config"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &cfg) != nil {
		return 0
	}
	sec := cfg.Config.WSReconnectIntervalSec
	if sec <= 0 {
		return 0
	}
	return time.Duration(sec) * time.Second
}
