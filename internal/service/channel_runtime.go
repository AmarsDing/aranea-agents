package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/runtime"
	"aranea-agents/internal/outbound"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const defaultRuntimeReloadInterval = 2 * time.Minute

// ChannelRuntime supervises long-lived channel connectors (MuseBot StartRobot equivalent).
type ChannelRuntime struct {
	mgr    *runtime.Manager
	lg     loggateway.Logger
	cancel context.CancelFunc
}

func NewChannelRuntime(channels *biz.ChannelUsecase, ingress *ChannelIngress, leases biz.ChannelRuntimeLeaseRepo, router *outbound.Router, lg loggateway.Logger) *ChannelRuntime {
	if channels == nil || ingress == nil {
		return nil
	}
	lookup := func(ctx context.Context, creds []biz.ChannelCredential, key string) (string, error) {
		return resolveCredentialPlain(ctx, channels, creds, key)
	}
	mgr := runtime.NewManager(channels, ingress, lookup, lg, router)
	if leases != nil {
		mgr = mgr.WithRuntimeLease(leases, channelRuntimeOwnerID(), 0)
	}
	return &ChannelRuntime{
		mgr: mgr,
		lg:  lg,
	}
}

func channelRuntimeOwnerID() string {
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s:%d", host, os.Getpid())
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
		if err := r.mgr.Reload(ctx); err != nil {
			r.lg.Warn("渠道运行时 Reload 失败",
				loggateway.StepID("channel.runtime.reload_failed"),
				loggateway.Err(err),
			)
		}
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
				if err := r.mgr.Reload(ctx); err != nil {
					r.lg.Warn("渠道运行时 Reload 失败",
						loggateway.StepID("channel.runtime.reload_failed"),
						loggateway.Err(err),
					)
				}
			}
		}
	})
}

// Reload restarts connectors after channel CRUD.
func (r *ChannelRuntime) Reload(ctx context.Context) {
	if r == nil || r.mgr == nil || ChannelRuntimeDisabled() {
		return
	}
	if err := r.mgr.Reload(ctx); err != nil {
		r.lg.Warn("渠道运行时 Reload 失败",
			loggateway.StepID("channel.runtime.reload_failed"),
			loggateway.Err(err),
		)
	}
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
