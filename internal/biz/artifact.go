package biz

import "aranea-agents/internal/biz/artifact"

// Re-export artifact types from sub-package for backward compatibility.
type (
	Artifact       = artifact.Artifact
	ArtifactRepo   = artifact.Repo
	ArtifactUsecase = artifact.Usecase
)

// Re-export artifact constructor and helpers for backward compatibility.
var (
	NewArtifactUsecase = artifact.NewUsecase
	NewArtifactID      = artifact.NewArtifactID
)
