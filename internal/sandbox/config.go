package sandbox

import "time"

// Config is the resolved sandbox subsystem configuration (design §7).
// Values are mapped from conf.Sandbox by the wire provider; env overrides
// are applied there as well. The zero Config is valid: DefaultConfig fills it.
type Config struct {
	Enabled  bool
	Pool     PoolConfig
	Limits   LimitsConfig
	TTL      TTLConfig
	Profiles map[string]Profile
	Egress   EgressConfig

	// DefaultProfile is used when AcquireReq.Profile is empty.
	DefaultProfile string

	// GCInterval overrides the lifecycle scan tick (0 = 30s).
	GCInterval time.Duration
}

// EgressConfig is the controlled-egress lane (P2-1). Profiles with
// network=egress get the CONNECT proxy injected as HTTP(S)_PROXY. The domain
// whitelist is held proxy-side only: docker/config/egress/{squid.conf,
// allowed_domains.txt} mounted into the egress-proxy container — single
// source of truth.
type EgressConfig struct {
	Network   string // per-sandbox internal network name PREFIX (default "aranea-egress"; actual networks are <prefix>-<sandboxID>, created/reaped per sandbox)
	ProxyHTTP string // proxy URL injected as HTTP(S)_PROXY (default "http://aranea-egress-proxy:3128")
}

type PoolConfig struct {
	MinReady          int           // per-profile warm target (default 2)
	MaxReady          int           // per-profile warm ceiling (default 8)
	ReplenishInterval time.Duration //补水 tick (default 5s)
	MaxPoolAge        time.Duration // ready-instance rotation age (default 10m)
}

type LimitsConfig struct {
	GlobalMaxActive   int // default 32
	PerAgentMaxActive int // default 4
	PerRunMaxCreate   int // P2: wired into team run budget chain
}

type TTLConfig struct {
	Default     time.Duration // lease TTL when req.TTL==0 (default 30m)
	Max         time.Duration // hard cap incl. Renew extensions (default 2h)
	IdleTimeout time.Duration // leased idle eviction (default 10m)
}

// DefaultProfileName is the built-in codeexec portrait (M32 parity).
const DefaultProfileName = "codeexec"

// DefaultConfig returns production-safe defaults (design §7).
func DefaultConfig() Config {
	return Config{
		Enabled: true,
		Pool: PoolConfig{
			MinReady:          2,
			MaxReady:          8,
			ReplenishInterval: 5 * time.Second,
			MaxPoolAge:        10 * time.Minute,
		},
		Limits: LimitsConfig{
			GlobalMaxActive:   32,
			PerAgentMaxActive: 4,
			PerRunMaxCreate:   16,
		},
		TTL: TTLConfig{
			Default:     30 * time.Minute,
			Max:         2 * time.Hour,
			IdleTimeout: 10 * time.Minute,
		},
		Egress: EgressConfig{
			Network:   "aranea-egress",
			ProxyHTTP: "http://aranea-egress-proxy:3128",
		},
		DefaultProfile: DefaultProfileName,
		Profiles: map[string]Profile{
			DefaultProfileName: {
				Name:           DefaultProfileName,
				Image:          "aranea-sandbox-base:local", // P1-3 基座镜像（docker/sandbox-base，build-sandbox-base.ps1 构建）
				CPUs:           0.5,
				MemoryBytes:    256 * 1024 * 1024,
				PidsLimit:      256,
				Network:        NetworkNone,
				ReadOnlyRootfs: true,
				TmpSize:        "128m",
			},
		},
		GCInterval: 30 * time.Second,
	}
}

// normalize fills zero fields with defaults and resolves profile specs.
func (c Config) normalize() Config {
	d := DefaultConfig()
	if c.Pool.MinReady <= 0 {
		c.Pool.MinReady = d.Pool.MinReady
	}
	if c.Pool.MaxReady <= 0 {
		c.Pool.MaxReady = d.Pool.MaxReady
	}
	if c.Pool.ReplenishInterval <= 0 {
		c.Pool.ReplenishInterval = d.Pool.ReplenishInterval
	}
	if c.Pool.MaxPoolAge <= 0 {
		c.Pool.MaxPoolAge = d.Pool.MaxPoolAge
	}
	if c.Limits.GlobalMaxActive <= 0 {
		c.Limits.GlobalMaxActive = d.Limits.GlobalMaxActive
	}
	if c.Limits.PerAgentMaxActive <= 0 {
		c.Limits.PerAgentMaxActive = d.Limits.PerAgentMaxActive
	}
	if c.Limits.PerRunMaxCreate <= 0 {
		c.Limits.PerRunMaxCreate = d.Limits.PerRunMaxCreate
	}
	if c.TTL.Default <= 0 {
		c.TTL.Default = d.TTL.Default
	}
	if c.TTL.Max <= 0 {
		c.TTL.Max = d.TTL.Max
	}
	if c.TTL.IdleTimeout <= 0 {
		c.TTL.IdleTimeout = d.TTL.IdleTimeout
	}
	if c.DefaultProfile == "" {
		c.DefaultProfile = d.DefaultProfile
	}
	if c.GCInterval <= 0 {
		c.GCInterval = d.GCInterval
	}
	if c.Egress.Network == "" {
		c.Egress.Network = d.Egress.Network
	}
	if c.Egress.ProxyHTTP == "" {
		c.Egress.ProxyHTTP = d.Egress.ProxyHTTP
	}
	if len(c.Profiles) == 0 {
		c.Profiles = d.Profiles
	} else {
		out := make(map[string]Profile, len(c.Profiles))
		for name, p := range c.Profiles {
			p.Name = name
			out[name] = p.withDefaults()
		}
		if _, ok := out[c.DefaultProfile]; !ok {
			// The default profile must always resolve; fall back to the built-in.
			out[c.DefaultProfile] = d.Profiles[DefaultProfileName]
		}
		c.Profiles = out
	}
	// P2 invariants (apply to built-in defaults too):
	for name, p := range c.Profiles {
		switch p.Network {
		case NetworkFull:
			// P2-4 fail-closed: full-network profiles are always
			// confirmation-gated, regardless of config input.
			if !p.RequiresConfirmation {
				p.RequiresConfirmation = true
				c.Profiles[name] = p
			}
		case NetworkEgress:
			// P2-1: resolve the egress lane onto the profile (engine consumes
			// profile fields only).
			if p.EgressNetwork == "" || p.EgressProxy == "" {
				p.EgressNetwork = c.Egress.Network
				p.EgressProxy = c.Egress.ProxyHTTP
				c.Profiles[name] = p
			}
		}
	}
	return c
}
