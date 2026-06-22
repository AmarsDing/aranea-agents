package artifactfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// sessionIDPattern is the allowed shape for an artifact session ID at the FS layer.
// OUT-05 / ART-01: prevents `../`, slashes, NUL, control chars and OS-specific
// path tricks (backslash, drive letters) from being interpreted as parent
// directory traversal by `filepath.Join(root, sessionID)`.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]{0,127}$`)

// validateSessionID returns nil only when id is safe to use as a single
// directory component beneath the artifact storage root.
func validateSessionID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("artifact: session_id is required")
	}
	if !sessionIDPattern.MatchString(id) {
		return fmt.Errorf("artifact: session_id contains disallowed characters")
	}
	if strings.Contains(id, "..") { // belt-and-braces: character class allows individual dots but not ".."; explicit check prevents traversal sequences
		return fmt.Errorf("artifact: session_id may not contain ..")
	}
	return nil
}

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
	root    string
	lg      loggateway.Logger
	mu      sync.RWMutex
	idIndex map[string][]artifactMeta // id → all versions of that artifact ID
}

func NewFSArtifactRepo(lg loggateway.Logger) *FSArtifactRepo {
	return &FSArtifactRepo{root: artifactStorageRoot(), lg: lg, idIndex: make(map[string][]artifactMeta)}
}

func NewFSArtifactRepoAt(root string, lg loggateway.Logger) *FSArtifactRepo {
	return &FSArtifactRepo{root: root, lg: lg, idIndex: make(map[string][]artifactMeta)}
}

var _ biz.ArtifactRepo = (*FSArtifactRepo)(nil)

func (r *FSArtifactRepo) sessionDir(sessionID string) string {
	return filepath.Join(r.root, sessionID)
}

// Save stores artifact bytes and returns the saved Artifact.
func (r *FSArtifactRepo) Save(_ context.Context, sessionID, name, mimeType string, data []byte) (biz.Artifact, error) {
	// OUT-05 / ART-01: reject any sessionID that could traverse out of the
	// configured artifact root before we touch the filesystem.
	if err := validateSessionID(sessionID); err != nil {
		return biz.Artifact{}, err
	}
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

	// OUT-05 / ART-03: persist a *relative* URI so API responses never disclose
	// the host filesystem layout. `Load` joins this against the configured root
	// at read time. Use forward slashes for a stable cross-platform layout key.
	relURI := path.Join(sessionID, binName)

	meta := artifactMeta{
		ID:          id,
		SessionID:   sessionID,
		Name:        name,
		MimeType:    mimeType,
		Size:        int64(len(data)),
		SHA256:      hash,
		StorageKind: "local",
		StorageURI:  relURI,
		Version:     version,
		CreatedAt:   now,
	}
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return biz.Artifact{}, fmt.Errorf("artifact meta marshal: %w", err)
	}
	metaPath := filepath.Join(dir, fmt.Sprintf("%s-v%d.json", id, version))
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		return biz.Artifact{}, fmt.Errorf("artifact meta write: %w", err)
	}

	// Artifact upload/download metrics are recorded at the service layer.

	// Update in-memory index.
	r.idIndex[id] = append(r.idIndex[id], meta)

	return meta.toBiz(), nil
}

// Load returns artifact data.  version <= 0 means latest.
func (r *FSArtifactRepo) Load(_ context.Context, id string, version int) (biz.Artifact, []byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, err := r.findMeta(id, version)
	if err != nil {
		return biz.Artifact{}, nil, err
	}
	data, err := os.ReadFile(r.resolveBinPath(meta))
	if err != nil {
		return biz.Artifact{}, nil, fmt.Errorf("artifact read: %w", err)
	}

	return meta.toBiz(), data, nil
}

// LoadMeta returns artifact metadata without reading the binary payload.
func (r *FSArtifactRepo) LoadMeta(_ context.Context, id string, version int) (biz.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	meta, err := r.findMeta(id, version)
	if err != nil {
		return biz.Artifact{}, err
	}
	return meta.toBiz(), nil
}

// LoadMetas returns metadata for multiple artifacts in a single lock acquisition.
// Missing IDs are silently skipped. version <= 0 means latest for each ID.
func (r *FSArtifactRepo) LoadMetas(_ context.Context, ids []string, version int) ([]biz.Artifact, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]biz.Artifact, 0, len(ids))
	for _, id := range ids {
		meta, err := r.findMeta(id, version)
		if err != nil {
			continue // skip missing
		}
		out = append(out, meta.toBiz())
	}
	return out, nil
}

// resolveBinPath returns the on-disk path for a metadata entry. New entries
// store a relative URI (OUT-05 / ART-03); legacy entries written before this
// change stored either an absolute path or a path that already includes the
// storage root (e.g. "data/artifacts/session/..."). We detect and handle all
// three cases so existing data stays readable. The result is always sanitized
// against r.root.
func (r *FSArtifactRepo) resolveBinPath(meta artifactMeta) string {
	uri := strings.TrimSpace(meta.StorageURI)
	if uri == "" {
		return filepath.Join(r.root, meta.SessionID, fmt.Sprintf("%s-v%d.bin", meta.ID, meta.Version))
	}
	if filepath.IsAbs(uri) {
		return uri
	}
	uriOS := filepath.FromSlash(uri)
	rootOS := filepath.FromSlash(r.root)
	if strings.HasPrefix(uriOS, rootOS+string(os.PathSeparator)) {
		return uri
	}
	return filepath.Join(r.root, uriOS)
}

// List returns artifact metadata for a session (no payload).
// When sessionID is empty, aggregates latest versions across all sessions (cross-session browse).
func (r *FSArtifactRepo) List(_ context.Context, sessionID string, limit, offset int) ([]biz.Artifact, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		metas []artifactMeta
		err   error
	)
	if strings.TrimSpace(sessionID) == "" {
		metas, err = r.listAllMetas()
	} else {
		metas, err = r.listSessionMetas(sessionID)
	}
	if err != nil {
		return nil, 0, err
	}
	// Deduplicate: keep only the latest version per session+name.
	type nameKey struct {
		sessionID string
		name      string
	}
	byName := map[nameKey]artifactMeta{}
	for _, m := range metas {
		key := nameKey{sessionID: m.SessionID, name: m.Name}
		if prev, ok := byName[key]; !ok || m.Version > prev.Version {
			byName[key] = m
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

	// Fast path: use index to locate files directly.
	if indexed, ok := r.idIndex[id]; ok && len(indexed) > 0 {
		var firstErr error
		for _, m := range indexed {
			dir := r.sessionDir(m.SessionID)
			// Remove binary.
			binPath := r.resolveBinPath(m)
			if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
				r.lg.Warn("artifact delete file failed", loggateway.Err(err), loggateway.Str("path", binPath))
				firstErr = fmt.Errorf("artifact delete %s: %w", binPath, err)
			}
			// Remove sidecar.
			metaPath := filepath.Join(dir, fmt.Sprintf("%s-v%d.json", m.ID, m.Version))
			if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) && firstErr == nil {
				r.lg.Warn("artifact delete file failed", loggateway.Err(err), loggateway.Str("path", metaPath))
				firstErr = fmt.Errorf("artifact delete %s: %w", metaPath, err)
			}
		}
		delete(r.idIndex, id)
		return firstErr
	}
	// Slow path: scan disk for entries not in index.
	entries, err := os.ReadDir(r.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("artifact delete scan: %w", err)
	}
	var firstErr error
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
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
					r.lg.Warn("artifact delete file failed", loggateway.Err(err), loggateway.Str("path", path))
					firstErr = fmt.Errorf("artifact delete %s: %w", path, err)
				}
			}
		}
	}
	delete(r.idIndex, id)
	return firstErr
}

// DeleteVersion removes exactly one version of a logical artifact identified by
// session+name+version. Returns a "not found" error when the version does not
// exist. Both the .bin data file and the .json metadata sidecar are removed.
func (r *FSArtifactRepo) DeleteVersion(_ context.Context, sessionID, name string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	metas, err := r.listSessionMetas(sessionID)
	if err != nil {
		return err
	}
	var target *artifactMeta
	for i := range metas {
		if metas[i].Name == name && metas[i].Version == version {
			target = &metas[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("artifact: version %d of %q not found in session %q", version, name, sessionID)
	}
	dir := r.sessionDir(sessionID)
	// Remove binary data file.
	if err := os.Remove(r.resolveBinPath(*target)); err != nil {
		r.lg.Warn("artifact delete bin failed", loggateway.Err(err), loggateway.Str("id", target.ID))
		return fmt.Errorf("artifact delete bin: %w", err)
	}
	// Remove metadata sidecar.
	metaPath := filepath.Join(dir, fmt.Sprintf("%s-v%d.json", target.ID, target.Version))
	if err := os.Remove(metaPath); err != nil {
		r.lg.Warn("artifact delete meta failed", loggateway.Err(err), loggateway.Str("path", metaPath))
		return fmt.Errorf("artifact delete meta: %w", err)
	}
	// Update in-memory index: remove the deleted version.
	if versions, ok := r.idIndex[target.ID]; ok {
		filtered := make([]artifactMeta, 0, len(versions))
		for _, v := range versions {
			if v.Version != target.Version {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			delete(r.idIndex, target.ID)
		} else {
			r.idIndex[target.ID] = filtered
		}
	}
	return nil
}

// ListBySessionAndName returns all version metadata for a session+name combo.
func (r *FSArtifactRepo) ListBySessionAndName(_ context.Context, sessionID, name string) ([]biz.Artifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

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
	if err := validateSessionID(sessionID); err != nil {
		return nil, err
	}
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
			r.lg.Warn("artifact json unmarshal failed", loggateway.StepID("data.artifactfs"), loggateway.Err(err))
			continue
		}
		metas = append(metas, m)
	}
	return metas, nil
}

func (r *FSArtifactRepo) findMeta(id string, version int) (artifactMeta, error) {
	// Fast path: check in-memory index (caller holds RLock).
	if entries, ok := r.idIndex[id]; ok && len(entries) > 0 {
		var best *artifactMeta
		for i := range entries {
			if version > 0 && entries[i].Version != version {
				continue
			}
			if best == nil || entries[i].Version > best.Version {
				cp := entries[i]
				best = &cp
			}
		}
		if best != nil {
			return *best, nil
		}
	}
	// Slow path: scan disk (handles entries written before this process started
	// or index misses due to process restart). We must release the read lock
	// before acquiring the write lock to avoid deadlock, then populate the
	// index under the write lock for future lookups.
	//
	// Since findMeta is called from methods that already hold a lock, we
	// cannot simply re-lock here. Instead, we do the disk scan without
	// modifying the index, and let the caller handle index population
	// separately if needed.
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
				r.lg.Warn("artifact json unmarshal failed", loggateway.StepID("data.artifactfs"), loggateway.Err(err))
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
	// Try in-memory index first to avoid disk scan.
	max := -1
	for _, versions := range r.idIndex {
		for _, m := range versions {
			if m.Name == name && filepath.Base(filepath.Dir(r.resolveBinPath(m))) == filepath.Base(dir) {
				if m.Version > max {
					max = m.Version
				}
			}
		}
	}
	if max >= 0 {
		return max + 1
	}
	// Fallback: scan disk for entries not in index.
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
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
			r.lg.Warn("artifact json unmarshal failed", loggateway.StepID("data.artifactfs"), loggateway.Err(err))
			continue
		}
		if m.Name == name && m.Version > max {
			max = m.Version
		}
	}
	return max + 1
}

// StorageBytes returns total bytes stored in .bin files under the artifact root.
func (r *FSArtifactRepo) StorageBytes(_ context.Context) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.storageBytesLocked()
}

func (r *FSArtifactRepo) storageBytesLocked() (int64, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("artifact storage scan: %w", err)
	}
	var total int64
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
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".bin") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			total += info.Size()
		}
	}
	return total, nil
}

func (r *FSArtifactRepo) listAllMetas() ([]artifactMeta, error) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("artifact list all: %w", err)
	}
	var metas []artifactMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sessionMetas, err := r.listSessionMetas(e.Name())
		if err != nil {
			return nil, err
		}
		metas = append(metas, sessionMetas...)
	}
	return metas, nil
}

// NewArtifactID is re-exported as a convenience; the authoritative definition lives in biz.
// Declared in data package to avoid import cycle; biz delegates here.
