// Package artifact implements artifact storage workflows.
package artifact

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// Domain errors — the Service layer maps these to kerrors.
var (
	// ErrIDRequired is returned when a required artifact ID is empty.
	ErrIDRequired = kerrors.BadRequest("ARTIFACT", "id is required")
)

// Artifact represents a stored binary artifact associated with a session.
type Artifact struct {
	ID          string
	SessionID   string
	Name        string
	MimeType    string
	Size        int64
	SHA256      string
	StorageKind string
	StorageURI  string
	Version     int
	CreatedAt   string
}

// Reader is the read-only persistence interface for artifact metadata + bytes.
type Reader interface {
	// Load returns artifact data.  version <= 0 means latest.
	Load(ctx context.Context, id string, version int) (Artifact, []byte, error)
	// LoadMeta returns artifact metadata without loading payload bytes. version <= 0 means latest.
	LoadMeta(ctx context.Context, id string, version int) (Artifact, error)
	// List returns artifact metadata for a session (no data payload).
	List(ctx context.Context, sessionID string, limit, offset int) ([]Artifact, int, error)
	// ListBySessionAndName lists all version metadata for a session+name combo.
	ListBySessionAndName(ctx context.Context, sessionID, name string) ([]Artifact, error)
}

// Writer is the write persistence interface for artifact metadata + bytes.
type Writer interface {
	// Save stores artifact bytes and returns the saved Artifact (with ID, version, SHA256, etc.).
	Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (Artifact, error)
	// Delete removes all versions of an artifact by ID.
	Delete(ctx context.Context, id string) error
	// DeleteVersion removes a single version identified by session+name+version.
	DeleteVersion(ctx context.Context, sessionID, name string, version int) error
}

// Repo is the combined persistence interface for artifact metadata + bytes.
// It composes Reader and Writer for backward compatibility.
// New code should depend on Reader or Writer directly when only one aspect is needed.
type Repo interface {
	Reader
	Writer
}

// Usecase wraps artifact Repo.
type Usecase struct {
	repo Repo
	lg   loggateway.Logger
}

// NewUsecase constructs an artifact Usecase.
func NewUsecase(repo Repo, lg loggateway.Logger) *Usecase {
	return &Usecase{repo: repo, lg: lg}
}

// Save stores an artifact.
func (uc *Usecase) Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (Artifact, error) {
	if err := ValidateUploadSize(int64(len(data))); err != nil {
		return Artifact{}, err
	}
	saved, err := uc.repo.Save(ctx, sessionID, name, mimeType, data)
	if err != nil {
		return Artifact{}, err
	}
	if c := CollectorFromContext(ctx); c != nil {
		c.Add(saved)
	}
	return saved, nil
}

// Load retrieves artifact data.  version <= 0 returns the latest version.
func (uc *Usecase) Load(ctx context.Context, id string, version int) (Artifact, []byte, error) {
	return uc.repo.Load(ctx, id, version)
}

// LoadMeta retrieves artifact metadata without reading artifact bytes.
func (uc *Usecase) LoadMeta(ctx context.Context, id string, version int) (Artifact, error) {
	return uc.repo.LoadMeta(ctx, id, version)
}

// List returns artifact metadata for a session. query and mimePrefix filter in-memory when set.
func (uc *Usecase) List(ctx context.Context, sessionID string, limit, offset int, query, mimePrefix string) ([]Artifact, int, error) {
	needsFilter := strings.TrimSpace(query) != "" || strings.TrimSpace(mimePrefix) != ""
	fetchLimit, fetchOffset := limit, offset
	if needsFilter {
		fetchLimit, fetchOffset = 0, 0
	}
	items, total, err := uc.repo.List(ctx, sessionID, fetchLimit, fetchOffset)
	if err != nil {
		return nil, 0, err
	}
	if !needsFilter {
		return items, total, nil
	}
	items = FilterArtifacts(items, query, mimePrefix)
	total = len(items)
	if offset >= total {
		return nil, total, nil
	}
	items = items[offset:]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// Delete removes an artifact and **all** its sibling versions (same session+name).
// DAT-01 / ART-04: each Save assigns a fresh ID, so a logical "artifact" (foo.txt v1/v2/v3)
// is materialized as three distinct IDs. The proto contract states DeleteArtifact removes
// "all versions"; we honor it here by resolving the session+name from the supplied ID,
// listing every version, and deleting each one. The caller's ID acts as a handle to the
// logical artifact, not a single version. To delete a specific version, use a future
// DeleteArtifactVersion RPC (not yet defined).
func (uc *Usecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	meta, _, err := uc.repo.Load(ctx, id, 0)
	if err != nil {
		// Best-effort: still attempt the legacy single-id delete so callers
		// can clean up an orphan whose meta is unreadable.
		if delErr := uc.repo.Delete(ctx, id); delErr != nil {
			uc.lg.Warn("best-effort delete failed for orphan artifact", loggateway.Err(delErr), loggateway.Str("id", id))
		}
		return err
	}
	versions, lvErr := uc.repo.ListBySessionAndName(ctx, meta.SessionID, meta.Name)
	if lvErr != nil || len(versions) == 0 {
		return uc.repo.Delete(ctx, id)
	}
	var firstErr error
	for _, v := range versions {
		if delErr := uc.repo.Delete(ctx, v.ID); delErr != nil && firstErr == nil {
			firstErr = delErr
		}
	}
	return firstErr
}

// DeleteVersion removes exactly one version of a logical artifact.
// id is any artifact ID belonging to the logical artifact (used to look up
// session+name). version must be > 0. Returns NotFound when the version does
// not exist, so callers can return HTTP 404 cleanly.
func (uc *Usecase) DeleteVersion(ctx context.Context, id string, version int) error {
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	if version <= 0 {
		return kerrors.BadRequest("ARTIFACT", "version must be > 0")
	}
	meta, _, err := uc.repo.Load(ctx, id, 0)
	if err != nil {
		return err
	}
	return uc.repo.DeleteVersion(ctx, meta.SessionID, meta.Name, version)
}

// ListVersions returns all version metadata for a session + filename.
func (uc *Usecase) ListVersions(ctx context.Context, sessionID, name string) ([]Artifact, error) {
	return uc.repo.ListBySessionAndName(ctx, sessionID, name)
}

// StorageBytes returns total stored bytes when the repo supports reporting.
func (uc *Usecase) StorageBytes(ctx context.Context) (int64, error) {
	type storageReporter interface {
		StorageBytes(context.Context) (int64, error)
	}
	r, ok := uc.repo.(storageReporter)
	if !ok {
		return 0, nil
	}
	return r.StorageBytes(ctx)
}

// PreviewKind describes how an artifact should be rendered in the browser.
type PreviewKind string

const (
	PreviewKindText   PreviewKind = "text"
	PreviewKindImage  PreviewKind = "image"
	PreviewKindPDF    PreviewKind = "pdf"
	PreviewKindBinary PreviewKind = "binary"
)

// PreviewResult holds the preview content for browser rendering.
type PreviewResult struct {
	Meta        Artifact
	Kind        PreviewKind
	TextContent string // populated when Kind == PreviewKindText
	Data        []byte // populated when Kind == PreviewKindImage or PreviewKindPDF
}

// maxTextPreviewBytes is the maximum bytes of text content returned in a preview.
const maxTextPreviewBytes = 512 << 10 // 512 KB

// Preview returns inline preview content for browser rendering.
// MIME classification and text truncation are business logic that belongs here,
// not in the Service layer.
func (uc *Usecase) Preview(ctx context.Context, id string, version int) (PreviewResult, error) {
	meta, data, err := uc.repo.Load(ctx, id, version)
	if err != nil {
		return PreviewResult{}, err
	}
	mime := strings.ToLower(strings.TrimSpace(meta.MimeType))
	result := PreviewResult{Meta: meta}
	switch {
	case strings.HasPrefix(mime, "text/") || mime == "application/json" || mime == "application/xml":
		result.Kind = PreviewKindText
		if len(data) > maxTextPreviewBytes {
			result.TextContent = string(data[:maxTextPreviewBytes]) + "\n…(truncated)"
		} else {
			result.TextContent = string(data)
		}
	case strings.HasPrefix(mime, "image/"):
		result.Kind = PreviewKindImage
		result.Data = data
	case mime == "application/pdf":
		result.Kind = PreviewKindPDF
		result.Data = data
	default:
		result.Kind = PreviewKindBinary
	}
	return result, nil
}

// NewArtifactID generates a random hex ID for a new artifact.
func NewArtifactID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte("artifact"))
	}
	return hex.EncodeToString(buf)
}
