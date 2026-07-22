package biz

import "aranea-agents/internal/biz/artifact"

// Re-export artifact types from sub-package for backward compatibility.
type (
	Artifact        = artifact.Artifact
	ArtifactRepo    = artifact.Repo
	ArtifactReader  = artifact.Reader
	ArtifactSaver   = artifact.Saver
	ArtifactWriter  = artifact.Writer
	ArtifactUsecase = artifact.Usecase
	PreviewKind     = artifact.PreviewKind
	PreviewResult   = artifact.PreviewResult
)

// Re-export artifact constructors, helpers and constants for backward compatibility.
var (
	NewArtifactUsecase = artifact.NewUsecase
	NewArtifactID      = artifact.NewArtifactID
)

// Re-export domain errors for Service-layer mapping.
var (
	ErrSizeExceeded            = artifact.ErrSizeExceeded
	ErrIDRequired              = artifact.ErrIDRequired
	ErrArtifactServiceRequired = artifact.ErrArtifactServiceRequired
	ErrAttachmentLoadFailed    = artifact.ErrAttachmentLoadFailed
	ErrAttachmentWrongSession  = artifact.ErrAttachmentWrongSession
)

const (
	PreviewKindText   = artifact.PreviewKindText
	PreviewKindImage  = artifact.PreviewKindImage
	PreviewKindPDF    = artifact.PreviewKindPDF
	PreviewKindBinary = artifact.PreviewKindBinary
)
