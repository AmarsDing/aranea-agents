package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/outbound"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// CredentialLookup resolves plain credential values from channel_credential rows.
type CredentialLookup func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error)

// Starter runs a long-lived platform connector until ctx is cancelled.
type Starter func(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup CredentialLookup, handler port.InboundHandler) error

type StarterWithLogger func(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup CredentialLookup, handler port.InboundHandler, lg loggateway.Logger) error

var (
	registryMu sync.RWMutex
	registry   = map[string]StarterWithLogger{}
)

func registryKey(channelType, receiveMode string) string {
	return strings.ToLower(strings.TrimSpace(channelType)) + "|" + strings.ToLower(strings.TrimSpace(receiveMode))
}

// RegisterStarter binds a channel type + receive_mode to a connector starter.
func RegisterStarter(channelType, receiveMode string, fn Starter) {
	wrapped := StarterWithLogger(func(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup CredentialLookup, handler port.InboundHandler, _ loggateway.Logger) error {
		return fn(ctx, ch, creds, lookup, handler)
	})
	registerStarterWithLogger(channelType, receiveMode, wrapped)
}

func RegisterStarterWithLogger(channelType, receiveMode string, fn StarterWithLogger) {
	registerStarterWithLogger(channelType, receiveMode, fn)
}

func registerStarterWithLogger(channelType, receiveMode string, fn StarterWithLogger) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[registryKey(channelType, receiveMode)] = fn
}

func lookupStarter(channelType, receiveMode string) (StarterWithLogger, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := registry[registryKey(channelType, receiveMode)]
	return fn, ok
}

const runtimeReplaceShutdownWait = 15 * time.Second
const runtimeLeaseTTL = 30 * time.Second

type runningInstance struct {
	cancel      context.CancelFunc
	fingerprint string
	done        chan struct{} // closed when runSupervised goroutine exits
}

// Manager supervises long-running channel connectors per DB instance.
type Manager struct {
	channels   *biz.ChannelUsecase
	handler    port.InboundHandler
	credLookup CredentialLookup
	lg         loggateway.Logger
	router     *outbound.Router

	mu      sync.Mutex
	running map[string]runningInstance

	// reloadMu serializes full Reload passes. Reload is triggered both by
	// async CRUD hooks (service reloadRuntime) and by the periodic
	// reconciler; concurrent passes would race stop/start of the same
	// connector.
	reloadMu sync.Mutex

	leaseRepo biz.ChannelRuntimeLeaseRepo
	ownerID   string
	leaseTTL  time.Duration

	// bus 为可选的监控事件总线，注入后启用连接生命周期流程日志
	// （channel.connect.open/close/error）；nil 时所有 EmitConnect* 帮助函数为 no-op。
	bus contract.MonitorBus
}

func NewManager(channels *biz.ChannelUsecase, handler port.InboundHandler, credLookup CredentialLookup, lg loggateway.Logger, router *outbound.Router) *Manager {
	return &Manager{
		channels:   channels,
		handler:    handler,
		credLookup: credLookup,
		lg:         lg,
		router:     router,
		running:    map[string]runningInstance{},
	}
}

func (m *Manager) WithRuntimeLease(repo biz.ChannelRuntimeLeaseRepo, ownerID string, ttl time.Duration) *Manager {
	if m == nil {
		return nil
	}
	m.leaseRepo = repo
	m.ownerID = strings.TrimSpace(ownerID)
	if ttl <= 0 {
		ttl = runtimeLeaseTTL
	}
	m.leaseTTL = ttl
	return m
}

// Router returns the outbound router, if configured.
func (m *Manager) Router() *outbound.Router {
	if m == nil {
		return nil
	}
	return m.router
}

// WithMonitorBus wires the monitor bus for connection-lifecycle flow logs.
// Nil bus keeps flow emission disabled (EmitConnect* helpers stay no-ops).
func (m *Manager) WithMonitorBus(bus contract.MonitorBus) *Manager {
	if m == nil {
		return nil
	}
	m.bus = bus
	return m
}

// Reload stops stale connectors and starts enabled runtime instances.
func (m *Manager) Reload(ctx context.Context) error {
	if m == nil || m.channels == nil || m.handler == nil {
		return nil
	}
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	m.lg.Info("Channel Runtime Reload 开始",
		loggateway.StepID("channel.runtime.reload_start"),
	)
	items, err := m.channels.List(ctx)
	if err != nil {
		return err
	}
	want := map[string]string{}
	appIDOwners := map[string]string{}
	for _, ch := range items {
		if !NeedsRuntimeConnector(ch, m.lg) {
			continue
		}
		cfg := ParseInstanceConfig(ch.ConfigJSON, m.lg)
		mode := EffectiveReceiveMode(ch, m.lg)
		if _, ok := lookupStarter(cfg.Type, mode); !ok {
			continue
		}
		if strings.EqualFold(cfg.Type, "feishu") {
			appID, appIDErr := feishuAppIDFromConfig(ch.ConfigJSON)
			if appIDErr != nil {
				m.lg.Warn("飞书 app_id 解析失败，跳过去重检查",
					loggateway.StepID("channel.runtime.feishu_appid_parse_failed"),
					loggateway.Str("channel_id", ch.ID),
					loggateway.Err(appIDErr),
				)
			} else if appID != "" {
				if owner, dup := appIDOwners[appID]; dup {
					m.lg.Warn("同 app_id 已有 enabled channel 占用 WS，跳过启动",
						loggateway.StepID("channel.runtime.app_id_conflict"),
						loggateway.Str("app_id", appID),
						loggateway.Str("channel_id", ch.ID),
						loggateway.Str("existing_channel_id", owner),
					)
					continue
				}
				appIDOwners[appID] = ch.ID
			}
		}
		creds, err := m.channels.ListCredentialsRaw(ctx, ch.ID)
		if err != nil {
			m.lg.Warn("Channel Runtime 读取凭据失败",
				loggateway.StepID("channel.runtime.credentials_fail"),
				loggateway.Str("channel_id", ch.ID),
				loggateway.Err(err),
			)
			continue
		}
		want[ch.ID] = runtimeFingerprint(ch, mode, CredentialsRevision(creds))
	}

	stopping := make([]runningInstance, 0)
	m.mu.Lock()
	for id, inst := range m.running {
		fp, keep := want[id]
		if !keep || fp != inst.fingerprint {
			inst.cancel()
			stopping = append(stopping, inst)
			delete(m.running, id)
		}
	}
	m.mu.Unlock()
	for _, inst := range stopping {
		waitRuntimeInstanceDone(inst.done, runtimeReplaceShutdownWait, m.lg)
	}

	started := 0
	for _, ch := range items {
		fp, ok := want[ch.ID]
		if !ok {
			continue
		}
		m.mu.Lock()
		if inst, already := m.running[ch.ID]; already && inst.fingerprint == fp {
			m.mu.Unlock()
			continue
		}
		runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		done := make(chan struct{})
		m.running[ch.ID] = runningInstance{cancel: cancel, fingerprint: fp, done: done}
		m.mu.Unlock()

		chCopy := ch
		cfg := ParseInstanceConfig(chCopy.ConfigJSON, m.lg)
		mode := EffectiveReceiveMode(chCopy, m.lg)
		st, _ := lookupStarter(cfg.Type, mode)
		platform := cfg.Type
		if !m.acquireLease(ctx, chCopy.ID, platform) {
			cancel()
			m.mu.Lock()
			delete(m.running, chCopy.ID)
			m.mu.Unlock()
			close(done)
			continue
		}
		safego.Go(runCtx, "channel.runtime.connector", func() {
			defer close(done)
			m.runSupervised(runCtx, chCopy, fp, st, platform, mode)
		})
		started++
		m.lg.Info("Channel Runtime 启动连接器",
			loggateway.StepID("channel.runtime.connector_start"),
			loggateway.Str("channel_id", chCopy.ID),
			loggateway.Str("platform", platform),
			loggateway.Str("receive_mode", mode),
			loggateway.Str("fingerprint", fp),
		)
	}
	m.lg.Info("Channel Runtime Reload 完成",
		loggateway.StepID("channel.runtime.reload_done"),
		loggateway.Int("want", len(want)),
		loggateway.Int("stopped", len(stopping)),
		loggateway.Int("started", started),
	)
	return nil
}

func (m *Manager) acquireLease(ctx context.Context, channelID, platform string) bool {
	if m == nil || m.leaseRepo == nil {
		return true
	}
	lease := biz.NewChannelRuntimeLease(channelID, platform, m.ownerID, m.leaseTTL, time.Now().UTC())
	claimed, err := m.leaseRepo.TryAcquireRuntimeLease(ctx, lease)
	if err != nil {
		m.lg.Warn("Channel Runtime 获取租约失败",
			loggateway.StepID("channel.runtime.lease_acquire_fail"),
			loggateway.Str("channel_id", channelID),
			loggateway.Str("platform", platform),
			loggateway.Err(err),
		)
		return false
	}
	if !claimed {
		m.lg.Info("Channel Runtime 租约被其他副本持有，跳过启动",
			loggateway.StepID("channel.runtime.lease_skip"),
			loggateway.Str("channel_id", channelID),
			loggateway.Str("platform", platform),
		)
	}
	return claimed
}

// runtimeFingerprint hashes only fields that should restart a connector.
// Do not include UpdatedAt: health-check metadata updates must not spawn duplicate WS clients.
func runtimeFingerprint(ch biz.Channel, mode, credRev string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		ch.ID,
		ch.ConfigJSON,
		fmt.Sprint(ch.Enabled),
		mode,
		credRev,
	}, "|")))
	return hex.EncodeToString(sum[:8])
}

func waitRuntimeInstanceDone(done chan struct{}, timeout time.Duration, lg loggateway.Logger) {
	if done == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		lg.Warn("Channel Runtime 旧连接器退出超时",
			loggateway.StepID("channel.runtime.connector_stop_timeout"),
			loggateway.Any("wait_ms", timeout.Milliseconds()),
		)
	}
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, id)
}

// StopAll cancels every running connector and waits for them to exit.
func (m *Manager) StopAll() {
	m.mu.Lock()
	instances := make([]runningInstance, 0, len(m.running))
	for id, inst := range m.running {
		inst.cancel()
		instances = append(instances, inst)
		delete(m.running, id)
	}
	m.mu.Unlock()
	for _, inst := range instances {
		waitRuntimeInstanceDone(inst.done, runtimeReplaceShutdownWait, m.lg)
	}
}

// ErrNoStarter indicates no runtime connector registered for type/mode.
var ErrNoStarter = apierror.BadRequest("CHANNEL_RUNTIME", "no starter registered")

func feishuAppIDFromConfig(configJSON string) (string, error) {
	var cfg struct {
		Config struct {
			AppID string `json:"app_id"`
		} `json:"config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(configJSON)), &cfg); err != nil {
		return "", fmt.Errorf("feishuAppIDFromConfig: parse config: %w", err)
	}
	return strings.TrimSpace(cfg.Config.AppID), nil
}
