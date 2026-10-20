package youtube_x

import (
	"context"

	conn "aranea-agents/internal/connector/publish"
)

// Adapter is a phased stub. MVP: package mode via generic package adapter only.
// Do NOT claim auto-publish to youtube_x.
type Adapter struct{}

func (a *Adapter) Name() string { return "youtube_x" }

func (a *Adapter) Capabilities() conn.Capabilities {
	return conn.Capabilities{SupportsAPI: false, SupportsPackage: true, Platform: "youtube_x"}
}

func (a *Adapter) Validate(context.Context, conn.Request) error { return conn.ErrAdapterPhaseNotReady }
func (a *Adapter) Schedule(context.Context, conn.Request) error { return conn.ErrAdapterPhaseNotReady }
func (a *Adapter) Publish(context.Context, conn.Request) (conn.Result, error) {
	return conn.Result{}, conn.ErrNotImplemented
}
func (a *Adapter) GetStatus(context.Context, string) (string, error) { return "", conn.ErrNotImplemented }
func (a *Adapter) Revoke(context.Context, string) error { return conn.ErrNotImplemented }
