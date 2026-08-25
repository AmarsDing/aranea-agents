package sandbox

import (
	"strings"

	"aranea-agents/internal/conf"
)

// ConfigFromProto maps the wire config (conf.Sandbox) onto the runtime
// Config. Zero/empty fields fall back to design defaults in normalize()
// (82-sandbox.design.md §7); kratos env source overrides land in the proto
// before this mapping, so SANDBOX__* env vars work transparently.
func ConfigFromProto(pb *conf.Sandbox) Config {
	cfg := DefaultConfig()
	if pb == nil {
		return cfg
	}
	cfg.Enabled = pb.GetEnabled()
	if p := pb.GetPool(); p != nil {
		cfg.Pool.MinReady = int(p.GetMinReady())
		cfg.Pool.MaxReady = int(p.GetMaxReady())
		if d := p.GetReplenishInterval(); d != nil {
			cfg.Pool.ReplenishInterval = d.AsDuration()
		}
		if d := p.GetMaxPoolAge(); d != nil {
			cfg.Pool.MaxPoolAge = d.AsDuration()
		}
	}
	if l := pb.GetLimits(); l != nil {
		cfg.Limits.GlobalMaxActive = int(l.GetGlobalMaxActive())
		cfg.Limits.PerAgentMaxActive = int(l.GetPerAgentMaxActive())
		cfg.Limits.PerRunMaxCreate = int(l.GetPerRunMaxCreate())
	}
	if t := pb.GetTtl(); t != nil {
		if d := t.GetDefault(); d != nil {
			cfg.TTL.Default = d.AsDuration()
		}
		if d := t.GetMax(); d != nil {
			cfg.TTL.Max = d.AsDuration()
		}
		if d := t.GetIdleTimeout(); d != nil {
			cfg.TTL.IdleTimeout = d.AsDuration()
		}
	}
	if len(pb.GetProfiles()) > 0 {
		profiles := make(map[string]Profile, len(pb.GetProfiles()))
		for name, pp := range pb.GetProfiles() {
			profiles[name] = Profile{
				Name:           name,
				Image:          pp.GetImage(),
				CPUs:           pp.GetCpus(),
				MemoryBytes:    pp.GetMemoryBytes(),
				PidsLimit:      pp.GetPidsLimit(),
				Network:        NetworkMode(strings.ToLower(strings.TrimSpace(pp.GetNetwork()))),
				ReadOnlyRootfs: true, // engine-enforced baseline, not configurable
				TmpSize:        pp.GetTmpSize(),
			}
		}
		cfg.Profiles = profiles
	}
	return cfg.normalize()
}
