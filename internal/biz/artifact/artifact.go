// Package artifact implements artifact storage workflows.
package artifact

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	return uc.repo.Save(ctx, sessionID, name, mimeType, data)
}

// Load retrieves artifact data.  version <= 0 returns the latest version.
func (uc *Usecase) Load(ctx context.Context, id string, version int) (Artifact, []byte, error) {
	return uc.repo.Load(ctx, id, version)
}

// List returns artifact metadata for a session.
func (uc *Usecase) List(ctx context.Context, sessionID string, limit, offset int) ([]Artifact, int, error) {
	return uc.repo.List(ctx, sessionID, limit, offset)
}

// Delete removes an artifact.
func (uc *Usecase) Delete(ctx context.Context, id string) error {
	return uc.repo.Delete(ctx, id)
}

// ListVersions returns all version metadata for a session + filename.
func (uc *Usecase) ListVersions(ctx context.Context, sessionID, name string) ([]Artifact, error) {
	return uc.repo.ListBySessionAndName(ctx, sessionID, name)
}

// NewArtifactID generates a random hex ID for a new artifact.
func NewArtifactID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte("artifact"))
	}
	return hex.EncodeToString(buf)
}
