package packagepub

import (
	"context"
	"path/filepath"

	bizpub "aranea-agents/internal/biz/publish"
	conn "aranea-agents/internal/connector/publish"
)

// Adapter implements Publish mode A: write Artifact package, do not call platform APIs.
type Adapter struct {
	Root string
}

func (a *Adapter) Name() string { return "package" }

func (a *Adapter) Capabilities() conn.Capabilities {
	return conn.Capabilities{SupportsAPI: false, SupportsPackage: true, Platform: "generic"}
}

func (a *Adapter) Validate(_ context.Context, req conn.Request) error {
	return bizpub.ValidateTitle(req.Title)
}

func (a *Adapter) Schedule(context.Context, conn.Request) error { return nil }

func (a *Adapter) Publish(_ context.Context, req conn.Request) (conn.Result, error) {
	if err := bizpub.ValidateTitle(req.Title); err != nil {
		return conn.Result{}, err
	}
	root := a.Root
	if req.PackageRoot != "" {
		root = req.PackageRoot
	}
	job := bizpub.Job{
		ID: req.JobID, Platform: req.Platform, Title: req.Title, Body: req.Body,
		Tags: req.Tags, MediaPaths: req.MediaPaths, ReviewTaskID: req.ReviewTaskID,
		ContentItemID: req.ContentItemID, Mode: bizpub.ModePackage,
	}
	if job.ID == "" {
		job.ID = "pkg"
	}
	path, _, err := bizpub.BuildPackage(root, job)
	if err != nil {
		return conn.Result{Status: bizpub.StatusFailed, Message: err.Error()}, err
	}
	return conn.Result{
		Status:     bizpub.StatusPackaged,
		ArtifactID: job.ID,
		PackageDir: filepath.Dir(path),
		Message:    "packaged_not_posted_to_platform",
	}, nil
}

func (a *Adapter) GetStatus(context.Context, string) (string, error) {
	return bizpub.StatusPackaged, nil
}

func (a *Adapter) Revoke(context.Context, string) error { return nil }
