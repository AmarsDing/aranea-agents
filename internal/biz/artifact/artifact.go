// Package artifact implements artifact storage workflows.
package artifact

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
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

// Repo is the persistence interface for artifact metadata + bytes.
type Repo interface {
	// Save stores artifact bytes and returns the saved Artifact (with ID, version, SHA256, etc.).
	Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (Artifact, error)
	// Load returns artifact data.  version <= 0 means latest.
	Load(ctx context.Context, id string, version int) (Artifact, []byte, error)
	// LoadMeta returns artifact metadata without loading payload bytes. version <= 0 means latest.
	LoadMeta(ctx context.Context, id string, version int) (Artifact, error)
	// List returns artifact metadata for a session (no data payload).
	List(ctx context.Context, sessionID string, limit, offset int) ([]Artifact, int, error)
	// Delete removes all versions of an artifact by ID.
	Delete(ctx context.Context, id string) error
	// DeleteVersion removes a single version identified by session+name+version.
	DeleteVersion(ctx context.Context, sessionID, name string, version int) error
	// ListBySessionAndName lists all version metadata for a session+name combo.
	ListBySessionAndName(ctx context.Context, sessionID, name string) ([]Artifact, error)
}

// Usecase wraps artifact Repo.
type Usecase struct {
	repo Repo
}

// NewUsecase constructs an artifact Usecase.
func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
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
		return uc.repo.Delete(ctx, id)
	}
	meta, _, err := uc.repo.Load(ctx, id, 0)
	if err != nil {
		// Best-effort: still attempt the legacy single-id delete so callers
		// can clean up an orphan whose meta is unreadable.
		_ = uc.repo.Delete(ctx, id)
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
		return errors.New("artifact: id is required")
	}
	if version <= 0 {
		return errors.New("artifact: version must be > 0")
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

// NewArtifactID generates a random hex ID for a new artifact.
func NewArtifactID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte("artifact"))
	}
	return hex.EncodeToString(buf)
}
