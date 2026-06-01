package modelregistry

import "time"

type LogoPhase struct{}

func NewLogoPhase() *LogoPhase { return &LogoPhase{} }

func (p *LogoPhase) Name() string         { return "logos" }
func (p *LogoPhase) Timeout() time.Duration { return 120 * time.Second }

func (p *LogoPhase) Run(pc *PhaseContext) PhaseResult {
	if len(pc.Directory) == 0 {
		return PhaseResult{PhaseName: "logos", Status: PhaseSkipped}
	}
	res := SyncProviderLogos(pc.Ctx, pc.Store, pc.Directory, defaultLogosBaseURL)
	status := PhaseSucceeded
	if res.Failed > 0 && res.Synced == 0 {
		status = PhaseFailed
	}
	return PhaseResult{
		PhaseName: "logos",
		Status:    status,
		Stats:     map[string]int{"synced": res.Synced, "failed": res.Failed, "removed": res.Removed},
		Errors:    res.Errors,
	}
}
