package publish

import (
	"context"
	"errors"
)

var (
	ErrNotImplemented      = errors.New("publish adapter: not implemented")
	ErrAdapterPhaseNotReady = errors.New("publish adapter: phase not ready (package mode only in MVP)")
)

// Capabilities describes what an adapter can do. MVP platforms: package=true, api=false.
type Capabilities struct {
	SupportsAPI     bool `json:"supports_api"`
	SupportsPackage bool `json:"supports_package"`
	Platform        string `json:"platform"`
}

// Request is a publish attempt (mode A writes package; API adapters return ErrNotImplemented).
type Request struct {
	JobID         string
	Platform      string
	Title         string
	Body          string
	Tags          []string
	MediaPaths    []string
	ReviewTaskID  string
	ContentItemID string
	PackageRoot   string
}

type Result struct {
	Status     string // packaged | failed
	ArtifactID string
	PackageDir string
	Message    string
}

// Connector is the publish port. Channel ≠ Publish.
type Connector interface {
	Name() string
	Capabilities() Capabilities
	Validate(ctx context.Context, req Request) error
	Schedule(ctx context.Context, req Request) error
	Publish(ctx context.Context, req Request) (Result, error)
	GetStatus(ctx context.Context, jobID string) (string, error)
	Revoke(ctx context.Context, jobID string) error
}
