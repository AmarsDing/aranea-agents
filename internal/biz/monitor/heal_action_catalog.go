package monitor

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ProviderProbeRunner refreshes LLM provider health states on demand.
// Satisfied by biz.LlmProviderModelUsecase.RunHealthChecks.
type ProviderProbeRunner interface {
	RunHealthChecks(ctx context.Context) error
}

// MCPHealthRefresher reprobes enabled MCP servers and refreshes their
// persisted health status. Satisfied by biz.MCPServerUsecase.RefreshEnabledHealth.
type MCPHealthRefresher interface {
	RefreshEnabledHealth(ctx context.Context, limit int) (probed int, err error)
}

// healActionMaxBackoff caps the sleep between retry attempts so a single heal
// action cannot stall the cron job for minutes.
const healActionMaxBackoff = 10 * time.Second

// defaultMCPRefreshLimit bounds how many MCP servers a reconnect action reprobes.
const defaultMCPRefreshLimit = 20

// CatalogHealActionHandler is the real HealActionHandler: it dispatches
// FixAction types to bound executors with actual side effects.
//
// Action catalog:
//   - retry:      re-run provider health checks (refresh health snapshot early)
//   - reconnect:  reprobe enabled MCP servers (refresh persisted health early)
//   - fallback:   record-only — strategies are consumed by the runtime, no
//     preventive side effect exists
//   - log_only:   record-only no-op
//
// Unknown action types fail closed (error) so misconfigured patterns surface
// as HealStatusFailed records instead of being silently dropped.
type CatalogHealActionHandler struct {
	lg        loggateway.Logger
	prober    ProviderProbeRunner
	refresher MCPHealthRefresher
}

// NewCatalogHealActionHandler creates an empty catalog. Bind executors via
// BindRetry / BindReconnect; unbound action types return an error when fired.
func NewCatalogHealActionHandler(lg loggateway.Logger) *CatalogHealActionHandler {
	return &CatalogHealActionHandler{lg: lg}
}

// BindRetry binds the executor for "retry" actions.
func (h *CatalogHealActionHandler) BindRetry(p ProviderProbeRunner) *CatalogHealActionHandler {
	if h != nil {
		h.prober = p
	}
	return h
}

// BindReconnect binds the executor for "reconnect" actions.
func (h *CatalogHealActionHandler) BindReconnect(r MCPHealthRefresher) *CatalogHealActionHandler {
	if h != nil {
		h.refresher = r
	}
	return h
}

// HandleFixAction implements HealActionHandler.
func (h *CatalogHealActionHandler) HandleFixAction(ctx context.Context, action FixAction, _ map[string]any) error {
	if h == nil {
		return apierror.Internal("MONITOR", "CatalogHealActionHandler is nil")
	}
	switch action.Type {
	case "retry":
		return h.execRetry(ctx, action)
	case "reconnect":
		return h.execReconnect(ctx)
	case "fallback", "log_only":
		// Record-only: fallback strategies (compress_and_retry, skip_skill)
		// are runtime-consumed hints; no preventive side effect exists.
		return nil
	default:
		return apierror.BadRequest("MONITOR", "unknown fix action type %q", action.Type)
	}
}

// execRetry refreshes the provider health snapshot, retrying internal probe
// failures with exponential backoff per action.Params (backoff_ms,
// backoff_factor). Provider-unhealthy verdicts are not errors here — the
// action's purpose is to refresh health state early, whatever the outcome.
func (h *CatalogHealActionHandler) execRetry(ctx context.Context, action FixAction) error {
	if h.prober == nil {
		return apierror.Internal("MONITOR", "retry action has no provider prober bound")
	}
	attempts := action.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	backoff := time.Duration(paramInt(action.Params, "backoff_ms", 1000)) * time.Millisecond
	factor := paramFloat(action.Params, "backoff_factor", 2.0)

	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			if err := sleepCtx(ctx, backoff); err != nil {
				return err
			}
			backoff = time.Duration(float64(backoff) * factor)
			if backoff > healActionMaxBackoff {
				backoff = healActionMaxBackoff
			}
		}
		if err := h.prober.RunHealthChecks(ctx); err != nil {
			lastErr = err
			h.lg.Warn("CatalogHealActionHandler: retry probe attempt failed",
				loggateway.StepID("monitor.heal_action_retry_fail"),
				loggateway.Int("attempt", i+1),
				loggateway.Err(err))
			continue
		}
		return nil
	}
	return apierror.Internal("MONITOR", "retry action exhausted %d attempts: %s", attempts, lastErr)
}

// execReconnect reprobes enabled MCP servers to refresh persisted health.
func (h *CatalogHealActionHandler) execReconnect(ctx context.Context) error {
	if h.refresher == nil {
		return apierror.Internal("MONITOR", "reconnect action has no MCP refresher bound")
	}
	probed, err := h.refresher.RefreshEnabledHealth(ctx, defaultMCPRefreshLimit)
	if err != nil {
		return err
	}
	h.lg.Info("CatalogHealActionHandler: reconnect refresh complete",
		loggateway.StepID("monitor.heal_action_reconnect"),
		loggateway.Int("probed", probed))
	return nil
}

func paramInt(params map[string]any, key string, fallback int) int {
	if params == nil {
		return fallback
	}
	switch v := params[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return fallback
}

func paramFloat(params map[string]any, key string, fallback float64) float64 {
	if params == nil {
		return fallback
	}
	switch v := params[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return fallback
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
