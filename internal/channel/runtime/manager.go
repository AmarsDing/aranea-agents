package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/port"
	"aranea-agents/internal/event"
)

// InboundHandler processes a normalized inbound event (implemented by service.ChannelIngress).
type InboundHandler interface {
	ProcessInbound(ctx context.Context, ch biz.Channel, ev port.InboundEvent) error
}

// CredentialLookup resolves plain credential values from channel_credential rows.
type CredentialLookup func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error)

// Starter runs a long-lived platform connector until ctx is cancelled.
type Starter func(ctx context.Context, ch biz.Channel, creds []biz.ChannelCredential, lookup CredentialLookup, handler InboundHandler) error

var (
	registryMu sync.RWMutex
	registry   = map[string]Starter{}
)

func registryKey(channelType, receiveMode string) string {
	return strings.ToLower(strings.TrimSpace(channelType)) + "|" + strings.ToLower(strings.TrimSpace(receiveMode))
}

// RegisterStarter binds a channel type + receive_mode to a connector starter.
func RegisterStarter(channelType, receiveMode string, fn Starter) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[registryKey(channelType, receiveMode)] = fn
}

func lookupStarter(channelType, receiveMode string) (Starter, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	fn, ok := registry[registryKey(channelType, receiveMode)]
	return fn, ok
}

const runtimeReplaceShutdownWait = 15 * time.Second

type runningInstance struct {
	cancel      context.CancelFunc
	fingerprint string
	done        chan struct{} // closed when runSupervised goroutine exits
}

// Manager supervises long-running channel connectors per DB instance.
type Manager struct {
	channels   *biz.ChannelUsecase
	handler    InboundHandler
	credLookup CredentialLookup

	mu      sync.Mutex
	running map[string]runningInstance
}

func NewManager(channels *biz.ChannelUsecase, handler InboundHandler, credLookup CredentialLookup) *Manager {
	return &Manager{
		channels:   channels,
		handler:    handler,
		credLookup: credLookup,
		running:    map[string]runningInstance{},
	}
}

// Reload stops stale connectors and starts enabled runtime instances.
func (m *Manager) Reload(ctx context.Context) error {
	if m == nil || m.channels == nil || m.handler == nil {
		return nil
	}
	items, err := m.channels.List(ctx)
	if err != nil {
		return err
	}
	want := map[string]string{}
	for _, ch := range items {
		if !NeedsRuntimeConnector(ch) {
			continue
		}
		cfg := ParseInstanceConfig(ch.ConfigJSON)
		mode := EffectiveReceiveMode(ch)
		if _, ok := lookupStarter(cfg.Type, mode); !ok {
			continue
		}
		creds, err := m.channels.ListCredentialsRaw(ctx, ch.ID)
		if err != nil {
			event.SysLogWarn("channel.runtime.credentials_fail", "Channel Runtime 读取凭据失败",
				event.P("channel_id", ch.ID),
				event.P("error", err.Error()),
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
		waitRuntimeInstanceDone(inst.done, runtimeReplaceShutdownWait)
	}

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
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		m.running[ch.ID] = runningInstance{cancel: cancel, fingerprint: fp, done: done}
		m.mu.Unlock()

		chCopy := ch
		cfg := ParseInstanceConfig(chCopy.ConfigJSON)
		mode := EffectiveReceiveMode(chCopy)
		st, _ := lookupStarter(cfg.Type, mode)
		platform := cfg.Type
		go func(starter Starter, fingerprint, plat, recvMode string) {
			defer close(done)
			m.runSupervised(runCtx, chCopy, fingerprint, starter, plat, recvMode)
		}(st, fp, platform, mode)
		event.SysLogInfo("channel.runtime.connector_start", "Channel Runtime 启动连接器",
			event.P("channel_id", chCopy.ID),
			event.P("platform", platform),
			event.P("receive_mode", mode),
			event.P("fingerprint", fp),
		)
	}
	return nil
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

func waitRuntimeInstanceDone(done chan struct{}, timeout time.Duration) {
	if done == nil {
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		event.SysLogWarn("channel.runtime.connector_stop_timeout", "Channel Runtime 旧连接器退出超时",
			event.P("wait_ms", timeout.Milliseconds()),
		)
	}
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, id)
}

// StopAll cancels every running connector.
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, inst := range m.running {
		inst.cancel()
		delete(m.running, id)
	}
}

// ErrNoStarter indicates no runtime connector registered for type/mode.
var ErrNoStarter = fmt.Errorf("channel runtime: no starter registered")
