package service

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/pkg/safego"
)

const defaultRuntimeReloadInterval = 2 * time.Minute

// ChannelRuntime supervises long-lived channel connectors (MuseBot StartRobot equivalent).
type ChannelRuntime struct {
	mgr    *runtime.Manager
	cancel context.CancelFunc
}

// NewChannelRuntime wires runtime.Manager to ChannelIngress.
func NewChannelRuntime(channels *biz.ChannelUsecase, ingress *ChannelIngress) *ChannelRuntime {
	if channels == nil || ingress == nil {
		return nil
	}
	lookup := func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error) {
		return resolveCredentialPlain(ctx, creds, key)
	}
	return &ChannelRuntime{
		mgr: runtime.NewManager(channels, ingress, lookup),
	}
}

// Disabled reports CHANNEL_RUNTIME_DISABLED=1.
func ChannelRuntimeDisabled() bool {
	return strings.TrimSpace(os.Getenv("CHANNEL_RUNTIME_DISABLED")) == "1"
}

// RuntimeReloadInterval returns periodic reconcile interval (0 disables periodic reload).
func RuntimeReloadInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("CHANNEL_RUNTIME_RELOAD_INTERVAL"))
	if raw == "" {
		return defaultRuntimeReloadInterval
	}
	if raw == "0" || strings.EqualFold(raw, "off") || strings.EqualFold(raw, "false") {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultRuntimeReloadInterval
	}
	return d
}

// Start launches connectors for all enabled runtime-mode channel instances.
func (r *ChannelRuntime) Start(parent context.Context) {
	if r == nil || r.mgr == nil || ChannelRuntimeDisabled() {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	safego.Go(ctx, "channel_runtime.reload_loop", func() {
		_ = r.mgr.Reload(ctx)
		interval := RuntimeReloadInterval()
		if interval <= 0 {
			<-ctx.Done()
			return
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = r.mgr.Reload(ctx)
			}
		}
	})
}

// Reload restarts connectors after channel CRUD.
func (r *ChannelRuntime) Reload(ctx context.Context) {
	if r == nil || r.mgr == nil || ChannelRuntimeDisabled() {
		return
	}
	_ = r.mgr.Reload(ctx)
}

// ConnectionInfo returns live connector state for a channel (F-01b).
func (r *ChannelRuntime) ConnectionInfo(channelID string) runtime.ConnectionInfo {
	if r == nil || r.mgr == nil {
		return runtime.ConnectionInfo{}
	}
	return runtime.GetConnectionInfo(channelID)
}

// Stop cancels all running connectors.
func (r *ChannelRuntime) Stop() {
	if r == nil {
		return
	}
	if r.cancel != nil {
		r.cancel()
	}
	if r.mgr != nil {
		r.mgr.StopAll()
	}
}
