// Package artifact implements artifact storage workflows.
package artifact

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	// List returns artifact metadata for a session (no data payload).
	List(ctx context.Context, sessionID string, limit, offset int) ([]Artifact, int, error)
	// Delete removes all versions of an artifact by ID.
	Delete(ctx context.Context, id string) error
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

// Delete removes an artifact.
func (uc *Usecase) Delete(ctx context.Context, id string) error {
	return uc.repo.Delete(ctx, id)
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
