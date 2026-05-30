package modelregistry

import "time"

type FetchPhase struct{}

func NewFetchPhase() *FetchPhase { return &FetchPhase{} }

func (p *FetchPhase) Name() string         { return "fetch" }
func (p *FetchPhase) Timeout() time.Duration { return 120 * time.Second }

func (p *FetchPhase) Run(pc *PhaseContext) PhaseResult {
	syncer := NewSyncer(pc.Store)
	out, err := syncer.Sync(pc.Ctx, SyncInput{})
	if err != nil {
		return PhaseResult{PhaseName: "fetch", Status: PhaseFailed, Errors: []string{err.Error()}}
	}
	if out.Status == "ok" && out.Meta.ETag != "" && out.Log.Message == "not modified (304)" {
		return PhaseResult{PhaseName: "fetch", Status: PhaseSkipped}
	}
	return PhaseResult{
		PhaseName: "fetch",
		Status:    PhaseSucceeded,
		Stats:     map[string]int{"providers": out.Meta.ProviderCount, "models": out.Meta.ModelCount},
	}
}
