package runtime

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	runtimeReconnectInitial = time.Second
	runtimeReconnectMax     = 5 * time.Minute
	runtimeLeaseRenewRetry  = 3
)

func (m *Manager) runSupervised(
	parentCtx context.Context,
	ch biz.Channel,
	fp string,
	starter StarterWithLogger,
	platform, mode string,
) {
	defer m.remove(ch.ID)
	runCtx := parentCtx
	cancelRun := func() {}
	leaseKey := biz.ChannelRuntimeLeaseKey(ch.ID, platform)
	if m.leaseRepo != nil && leaseKey != "" && strings.TrimSpace(m.ownerID) != "" {
		runCtx, cancelRun = context.WithCancel(parentCtx)
		renewCtx, stopRenew := context.WithCancel(runCtx)
		defer cancelRun()
		defer stopRenew()
		defer func() {
			_ = m.leaseRepo.ReleaseRuntimeLease(context.Background(), leaseKey, m.ownerID)
		}()
		safego.Go(renewCtx, "channel.runtime.lease_renew", func() {
			m.renewLeaseLoop(renewCtx, leaseKey, platform, cancelRun)
		})
	}
	arametrics.ChannelRuntimeConnected.WithLabelValues(platform, mode).Inc()
	defer arametrics.ChannelRuntimeConnected.WithLabelValues(platform, mode).Dec()

	backoff := wsReconnectInterval(ch.ConfigJSON)
	if backoff <= 0 {
		backoff = runtimeReconnectInitial
	}
	initialBackoff := backoff
	// Attach the connection-lifecycle flow emitter so platform starters can
	// emit channel.connect.* events; nil when the monitor bus is not wired.
	if flow := m.connectFlow(runCtx); flow != nil {
		runCtx = event.WithTraceEmitter(runCtx, flow)
	}
	for {
		if runCtx.Err() != nil {
			return
		}
		if !m.fingerprintMatches(ch.ID, fp) {
			return
		}
		setChannelConnection(ch.ID, true)
		creds, err := m.channels.ListCredentialsRaw(runCtx, ch.ID)
		if err != nil {
			setChannelConnection(ch.ID, false)
			return
		}
		runErr := starter(runCtx, ch, creds, m.credLookup, m.handler, m.lg)
		setChannelConnection(ch.ID, false)
		if runCtx.Err() != nil {
			// 正常停止（Reload 替换 / 进程关停）。
			EmitConnectClose(runCtx, platform, ch.ID, "渠道连接已断开（连接器停止）")
			m.lg.Info("渠道连接器已停止",
				loggateway.StepID("channel.runtime.connector_stop"),
				loggateway.Str("platform", platform),
				loggateway.Str("channel_id", ch.ID),
			)
			return
		}
		if !m.fingerprintMatches(ch.ID, fp) {
			return
		}
		if runErr != nil {
			// 异常退出：channel.connect.error 流程日志由平台 starter 在具体
			// 失败点发射（拨号失败/读循环异常/重连失败），此处只记进程日志。
			m.lg.Warn("渠道连接器异常退出",
				loggateway.StepID("channel.runtime.starter_exited"),
				loggateway.Str("platform", platform),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Err(runErr),
			)
		} else {
			// 远端正常关闭（如 Telegram updates 通道关闭），稍后重连。
			EmitConnectClose(runCtx, platform, ch.ID, "渠道连接已断开")
		}
		// Reset backoff after a successful run so that transient disconnects
		// don't accumulate to the max backoff ceiling.
		backoff = initialBackoff
		arametrics.ChannelRuntimeReconnectTotal.WithLabelValues(platform, mode, "disconnect").Inc()
		arametrics.ChannelRuntimeReconnectTotal.WithLabelValues(platform, mode, "attempt").Inc()

		// Add jitter to avoid thundering herd when multiple channels
		// disconnect simultaneously (e.g., platform-side restart).
		jitter := time.Duration(rand.Int64N(int64(backoff) / 2))
		sleepDuration := backoff/2 + jitter
		m.lg.Info("渠道连接器准备重连",
			loggateway.StepID("channel.runtime.connector_reconnect"),
			loggateway.Str("platform", platform),
			loggateway.Str("channel_id", ch.ID),
			loggateway.Int64("backoff_ms", sleepDuration.Milliseconds()),
		)
		timer := time.NewTimer(sleepDuration)
		select {
		case <-runCtx.Done():
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

func (m *Manager) renewLeaseLoop(ctx context.Context, leaseKey, platform string, cancelRun context.CancelFunc) {
	ttl := m.leaseTTL
	if ttl <= 0 {
		ttl = runtimeLeaseTTL
	}
	interval := ttl / 3
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			renewed, err := m.leaseRepo.RenewRuntimeLease(ctx, leaseKey, m.ownerID, time.Now().UTC().Add(ttl))
			if err != nil || !renewed {
				consecutiveFailures++
				if err != nil {
					arametrics.ChannelRuntimeReconnectTotal.WithLabelValues(platform, "lease", "renew_error").Inc()
				}
				if consecutiveFailures >= runtimeLeaseRenewRetry {
					m.lg.Warn("渠道租约续期连续失败，取消连接器",
						loggateway.StepID("channel.runtime.lease_renew_exhausted"),
						loggateway.Str("platform", platform),
						loggateway.Int("failures", consecutiveFailures),
					)
					cancelRun()
					return
				}
				m.lg.Warn("渠道租约续期失败，将重试",
					loggateway.StepID("channel.runtime.lease_renew_retry"),
					loggateway.Str("platform", platform),
					loggateway.Int("failures", consecutiveFailures),
				)
			} else {
				consecutiveFailures = 0
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
