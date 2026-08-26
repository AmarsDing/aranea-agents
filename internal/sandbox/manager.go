package sandbox

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// destroyTimeout bounds the engine teardown call; detached from the caller
// ctx so a cancelled caller (e.g. exec timeout) cannot abort teardown.
const destroyTimeout = 15 * time.Second

// Manager is the sandbox供给层 entrypoint (design §5.1): Acquire hands out
// exclusive leases over warm-pool or cold-created instances; every lease is
// destroyed — never recycled — on Release/TTL/idle/reconcile (ADR-82-2).
//
// Manager also implements biz.SandboxAdminPort for the admin API.
type Manager struct {
	cfg      Config
	engine   Engine
	registry *Registry
	quota    *Quota
	lg       loggateway.Logger
	st       *stats

	now func() time.Time // test hook

	startOnce sync.Once
	cancel    context.CancelFunc
}

// NewManager builds a Manager. engine may be nil when the daemon is
// unavailable: the manager stays constructible (wire graph intact) and every
// Acquire fails fast with ErrDisabled so consumers fall back (NFR-04).
func NewManager(cfg Config, engine Engine, lg loggateway.Logger) *Manager {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	cfg = cfg.normalize()
	return &Manager{
		cfg:      cfg,
		engine:   engine,
		registry: NewRegistry(),
		quota:    NewQuota(cfg.Limits.GlobalMaxActive, cfg.Limits.PerAgentMaxActive, cfg.Limits.PerRunMaxCreate),
		lg:       lg,
		st:       newStats(),
		now:      time.Now,
	}
}

// Start runs the startup reconcile then launches the pool replenisher and GC
// loops. It is idempotent; ctx cancellation stops all loops (app shutdown).
func (m *Manager) Start(ctx context.Context) {
	m.startOnce.Do(func() {
		if !m.cfg.Enabled || m.engine == nil {
			m.lg.Warn("sandbox manager disabled or engine unavailable — pool/gc not started",
				loggateway.StepID("sandbox.start"),
				loggateway.Bool("enabled", m.cfg.Enabled),
				loggateway.Bool("engine", m.engine != nil))
			return
		}
		ctx, cancel := context.WithCancel(ctx)
		m.cancel = cancel

		m.reconcileOnce(ctx)

		safego.Go(ctx, "sandbox.pool_replenisher", func() { m.poolLoop(ctx) })
		safego.Go(ctx, "sandbox.gc", func() { m.gcLoop(ctx) })
		m.lg.Info("sandbox manager started",
			loggateway.StepID("sandbox.start"),
			loggateway.Int("min_ready", m.cfg.Pool.MinReady),
			loggateway.Int("max_ready", m.cfg.Pool.MaxReady),
			loggateway.Int("global_quota", m.cfg.Limits.GlobalMaxActive),
			loggateway.Int("per_agent_quota", m.cfg.Limits.PerAgentMaxActive))
	})
}

// Available reports whether the manager can serve Acquire (enabled and an
// engine is bound). Consumers use it for availability-based fallback.
func (m *Manager) Available() bool {
	return m.cfg.Enabled && m.engine != nil
}

// Close stops the background loops. Live sandboxes are intentionally NOT
// destroyed here — process shutdown orphans them, and the next boot's
// reconcile pass reaps them (design §3.3).
func (m *Manager) Close() {
	if m.cancel != nil {
		m.cancel()
	}
}

// Acquire leases a sandbox for the requester (design §3.1).
func (m *Manager) Acquire(ctx context.Context, req AcquireReq) (*Lease, error) {
	start := m.now()
	if !m.cfg.Enabled || m.engine == nil {
		return nil, ErrDisabled
	}
	profileName := req.Profile
	if profileName == "" {
		profileName = m.cfg.DefaultProfile
	}
	profile, ok := m.cfg.Profiles[profileName]
	if !ok {
		return nil, ErrProfileUnknown
	}

	// P2-4: confirmation-gated profiles (network=full is force-marked in
	// normalize) reject unless the caller passed a confirmation chain.
	if profile.RequiresConfirmation && !req.Confirmed {
		m.st.acquireFail.Add(1)
		m.lg.Warn("sandbox acquire rejected: confirmation required",
			loggateway.StepID("sandbox.confirm_reject"),
			loggateway.Str("profile", profileName),
			loggateway.Str("agent_key", req.AgentKey),
			loggateway.Str("session_id", req.SessionID))
		return nil, ErrConfirmationRequired
	}

	if err := m.quota.Admit(req.AgentKey, req.RunID); err != nil {
		scope := QuotaScopeGlobal
		if qe, ok2 := err.(*QuotaError); ok2 {
			scope = qe.Scope
		}
		quotaRejectTotal.WithLabelValues(scope).Inc()
		m.st.quotaReject.inc(scope)
		m.st.acquireFail.Add(1)
		m.lg.Warn("sandbox acquire rejected by quota",
			loggateway.StepID("sandbox.quota_reject"),
			loggateway.Str("scope", scope),
			loggateway.Str("agent_key", req.AgentKey),
			loggateway.Str("profile", profileName))
		return nil, err
	}

	ttl := req.TTL
	if ttl <= 0 {
		ttl = m.cfg.TTL.Default
	}
	if ttl > m.cfg.TTL.Max {
		ttl = m.cfg.TTL.Max
	}
	deadline := start.Add(ttl)

	// Warm path: claim a ready pool instance.
	if e := m.registry.claimReady(profileName, req.AgentKey, req.SessionID, req.RunID, deadline); e != nil {
		acquireDuration.WithLabelValues("warm").Observe(m.now().Sub(start).Seconds())
		m.st.acquireWarm.Add(1)
		m.refreshGauges()
		return &Lease{m: m, id: e.view.SandboxID, profile: profileName}, nil
	}

	// Cold path: create a fresh instance.
	lease, err := m.createLeased(ctx, profile, req, deadline)
	if err != nil {
		m.quota.UndoAdmit(req.AgentKey, req.RunID)
		acquireDuration.WithLabelValues("fail").Observe(m.now().Sub(start).Seconds())
		m.st.acquireFail.Add(1)
		return nil, err
	}
	acquireDuration.WithLabelValues("cold").Observe(m.now().Sub(start).Seconds())
	m.st.acquireCold.Add(1)
	m.refreshGauges()
	return lease, nil
}

// createLeased builds a fresh instance and registers it as leased.
// The caller must hold the quota slot (released on error here).
func (m *Manager) createLeased(ctx context.Context, profile Profile, req AcquireReq, deadline time.Time) (*Lease, error) {
	sandboxID := "sbx-" + uuid.NewString()
	labels := map[string]string{
		LabelSandbox:  "1",
		LabelID:       sandboxID,
		LabelProfile:  profile.Name,
		LabelAgentKey: req.AgentKey,
		LabelSession:  req.SessionID,
		LabelRun:      req.RunID,
		LabelDeadline: strconv.FormatInt(deadline.Unix(), 10),
	}
	h, err := m.engine.Create(ctx, sandboxID, profile, labels)
	if err != nil {
		m.lg.Warn("sandbox cold create failed",
			loggateway.StepID("sandbox.create"),
			loggateway.Str("profile", profile.Name),
			loggateway.Str("agent_key", req.AgentKey),
			loggateway.Err(err))
		return nil, err
	}
	m.registry.register(&entry{
		view: LeaseView{
			SandboxID: sandboxID,
			Profile:   profile.Name,
			AgentKey:  req.AgentKey,
			SessionID: req.SessionID,
			RunID:     req.RunID,
			State:     StateLeased,
			CreatedAt: m.now(),
			Deadline:  deadline,
		},
		handle:     h,
		entryState: StateLeased,
		quotaHeld:  true,
	})
	return &Lease{m: m, id: sandboxID, profile: profile.Name}, nil
}

// destroy is the single convergence point for all four teardown paths
// (Release/TTL/idle/pool_evict/reconcile/force). The registry CAS makes it
// single-fire; the engine destroy runs on a detached context so a cancelled
// caller (e.g. exec timeout) cannot abort the teardown.
func (m *Manager) destroy(id, reason string) {
	e, ok := m.registry.transitionDestroying(id)
	if !ok {
		return // already gone or teardown in flight
	}
	cleanCtx, cancel := context.WithTimeout(context.Background(), destroyTimeout)
	defer cancel()
	_ = m.engine.Destroy(cleanCtx, e.handle)
	m.registry.remove(id)
	if e.quotaHeld {
		m.quota.Release(e.view.AgentKey)
	}
	destroyTotal.WithLabelValues(reason).Inc()
	m.st.destroy.inc(reason)
	m.refreshGauges()
	m.lg.Info("sandbox destroyed",
		loggateway.StepID("sandbox.destroy"),
		loggateway.Str("sandbox_id", id),
		loggateway.Str("profile", e.view.Profile),
		loggateway.Str("reason", reason),
		loggateway.Str("agent_key", e.view.AgentKey),
		loggateway.Str("session_id", e.view.SessionID),
		loggateway.Str("run_id", e.view.RunID),
		loggateway.Int64("exec_count", e.view.ExecCount))
}

// ReleaseRun drops the run's cumulative-creation budget counter (P2-2; team
// run end hook). Live instances are untouched — they still die by
// Release/TTL/idle.
func (m *Manager) ReleaseRun(runID string) {
	if m == nil || m.quota == nil || runID == "" {
		return
	}
	m.quota.ReleaseRun(runID)
}

// ForceKill destroys a live sandbox on operator request (reason=force).
func (m *Manager) ForceKill(id, reason, operator string) error {
	if !m.cfg.Enabled || m.engine == nil {
		return ErrDisabled
	}
	if _, ok := m.registry.get(id); !ok {
		return ErrNotFound
	}
	m.lg.Warn("sandbox force-killed",
		loggateway.StepID("sandbox.force_kill"),
		loggateway.Str("sandbox_id", id),
		loggateway.Str("operator", operator),
		loggateway.Str("kill_reason", reason))
	m.destroy(id, ReasonForce)
	return nil
}

// refreshGauges recomputes the ready/active water-level gauges from the
// registry (cheap at单主机 scale; authoritative by construction).
func (m *Manager) refreshGauges() {
	leased := m.registry.countLeasedByProfile()
	for name := range m.cfg.Profiles {
		poolReadyGauge.WithLabelValues(name).Set(float64(m.registry.countReady(name)))
		activeGauge.WithLabelValues(name).Set(float64(leased[name]))
	}
}


