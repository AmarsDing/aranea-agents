package artifactfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
)

// artifactDirEnv is the env var for the artifact storage root.
const artifactDirEnv = "ARTIFACT_STORAGE_DIR"

// defaultArtifactDir is the fallback directory if the env var is not set.
const defaultArtifactDir = "data/artifacts"

// artifactStorageRoot returns the configured storage root.
func artifactStorageRoot() string {
	if d := strings.TrimSpace(os.Getenv(artifactDirEnv)); d != "" {
		return d
	}
	return defaultArtifactDir
}

// artifactMeta is the on-disk metadata sidecar (<dir>/<session>/<id>.json).
type artifactMeta struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Name        string `json:"name"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	StorageKind string `json:"storage_kind"`
	StorageURI  string `json:"storage_uri"`
	Version     int    `json:"version"`
	CreatedAt   string `json:"created_at"`
}

func (m artifactMeta) toBiz() biz.Artifact {
	return biz.Artifact{
		ID:          m.ID,
		SessionID:   m.SessionID,
		Name:        m.Name,
		MimeType:    m.MimeType,
		Size:        m.Size,
		SHA256:      m.SHA256,
		StorageKind: m.StorageKind,
		StorageURI:  m.StorageURI,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt,
	}
}

// FSArtifactRepo implements biz.ArtifactRepo using the local filesystem.
// Layout: <root>/<session_id>/<artifact_id>-v<version>.bin  (binary)
//
//	<root>/<session_id>/<artifact_id>-v<version>.json (meta sidecar)
type FSArtifactRepo struct {
	root string
	mu   sync.Mutex
}

// NewFSArtifactRepo creates a new FSArtifactRepo.
func NewFSArtifactRepo() *FSArtifactRepo {
	return &FSArtifactRepo{root: artifactStorageRoot()}
}

// NewFSArtifactRepoAt creates a new FSArtifactRepo with a custom root (tests).
func NewFSArtifactRepoAt(root string) *FSArtifactRepo {
	return &FSArtifactRepo{root: root}
}

var _ biz.ArtifactRepo = (*FSArtifactRepo)(nil)

func (r *FSArtifactRepo) sessionDir(sessionID string) string {
	return filepath.Join(r.root, sessionID)
}

// Save stores artifact bytes and returns the saved Artifact.
func (r *FSArtifactRepo) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (biz.Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	dir := r.sessionDir(sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return biz.Artifact{}, fmt.Errorf("artifact mkdir: %w", err)
	}

	// Determine next version for this name.
	version := r.nextVersion(dir, name)

	id := biz.NewArtifactID()
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	now := time.Now().UTC().Format(time.RFC3339)

	binName := fmt.Sprintf("%s-v%d.bin", id, version)
	binPath := filepath.Join(dir, binName)
	if err := os.WriteFile(binPath, data, 0o644); err != nil {
		return biz.Artifact{}, fmt.Errorf("artifact write: %w", err)
	}

	meta := artifactMeta{
		ID:          id,
		SessionID:   sessionID,
		Name:        name,
		MimeType:    mimeType,
		Size:        int64(len(data)),
		SHA256:      hash,
		StorageKind: "local",
		StorageURI:  binPath,
		Version:     version,
		CreatedAt:   now,
	}
	metaBytes, _ := json.Marshal(meta)
	metaPath := filepath.Join(dir, fmt.Sprintf("%s-v%d.json", id, version))
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		return biz.Artifact{}, fmt.Errorf("artifact meta write: %w", err)
	}

	// Artifact upload/download metrics are recorded at the service layer.

	return meta.toBiz(), nil
}

// Load returns artifact data.  version <= 0 means latest.
func (r *FSArtifactRepo) Load(_ context.Context, id string, version int) (biz.Artifact, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta, err := r.findMeta(id, version)
	if err != nil {
		return biz.Artifact{}, nil, err
	}
	data, err := os.ReadFile(meta.StorageURI)
	if err != nil {
		return biz.Artifact{}, nil, fmt.Errorf("artifact read: %w", err)
	}

	return meta.toBiz(), data, nil
}

// List returns artifact metadata for a session (no payload).
func (r *FSArtifactRepo) List(_ context.Context, sessionID string, limit, offset int) ([]biz.Artifact, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	metas, err := r.listSessionMetas(sessionID)
	if err != nil {
		return nil, 0, err
	}
	// Deduplicate: keep only the latest version per name.
	byName := map[string]artifactMeta{}
	for _, m := range metas {
		if prev, ok := byName[m.Name]; !ok || m.Version > prev.Version {
			byName[m.Name] = m
		}
	}
	items := make([]biz.Artifact, 0, len(byName))
	for _, m := range byName {
		items = append(items, m.toBiz())
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	total := len(items)
	if offset >= total {
		return nil, total, nil
	}
	items = items[offset:]
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, total, nil
}

// Delete removes all versions of an artifact by ID.
func (r *FSArtifactRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Walk all session dirs to find the artifact.
	entries, err := os.ReadDir(r.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("artifact delete scan: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(r.root, e.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if strings.HasPrefix(f.Name(), id+"-") {
				path := filepath.Join(dir, f.Name())
				_ = os.Remove(path)
			}
		}
	}
	return nil
}

// ListBySessionAndName returns all version metadata for a session+name combo.
func (r *FSArtifactRepo) ListBySessionAndName(_ context.Context, sessionID, name string) ([]biz.Artifact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	metas, err := r.listSessionMetas(sessionID)
	if err != nil {
		return nil, err
	}
	var out []biz.Artifact
	for _, m := range metas {
		if m.Name == name {
			out = append(out, m.toBiz())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// --- internal helpers ---

func (r *FSArtifactRepo) listSessionMetas(sessionID string) ([]artifactMeta, error) {
	dir := r.sessionDir(sessionID)
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("artifact list: %w", err)
	}
	var metas []artifactMeta
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var m artifactMeta
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		metas = append(metas, m)
	}
	return metas, nil
}

func (r *FSArtifactRepo) findMeta(id string, version int) (artifactMeta, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return artifactMeta{}, fmt.Errorf("artifact root read: %w", err)
	}
	var best *artifactMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(r.root, e.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasPrefix(f.Name(), id+"-") || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
			if err != nil {
				continue
			}
			var m artifactMeta
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			if version > 0 && m.Version != version {
				continue
			}
			if best == nil || m.Version > best.Version {
				cp := m
				best = &cp
			}
		}
	}
	if best == nil {
		return artifactMeta{}, fmt.Errorf("artifact not found: %s", id)
	}
	return *best, nil
}

func (r *FSArtifactRepo) nextVersion(dir, name string) int {
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	max := -1
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var m artifactMeta
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		if m.Name == name && m.Version > max {
			max = m.Version
		}
	}
	return max + 1
}

// NewArtifactID is re-exported as a convenience; the authoritative definition lives in biz.
// Declared in data package to avoid import cycle; biz delegates here.
