package modelcatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirName          = "model-catalog"
	policyFile       = "policy.json"
	currentFile      = "current.json"
	metaFile         = "current.meta.json"
	syncLogsFile     = "sync-logs.jsonl"
	migrationCheckpointFile = "migration-checkpoint.json"
	logosDirName     = "logos"
)

// Store manages on-disk catalog files under {root}/data/model-catalog/.
type Store struct {
	root string
}

func NewStore(rootDir string) *Store {
	return &Store{root: strings.TrimSpace(rootDir)}
}

func (s *Store) Dir() string {
	if s.root == "" {
		return filepath.Join("data", dirName)
	}
	return filepath.Join(s.root, "data", dirName)
}

func (s *Store) ensureDir() error {
	return os.MkdirAll(s.Dir(), 0o755)
}

func (s *Store) PolicyPath() string  { return filepath.Join(s.Dir(), policyFile) }
func (s *Store) CurrentPath() string { return filepath.Join(s.Dir(), currentFile) }
func (s *Store) MetaPath() string    { return filepath.Join(s.Dir(), metaFile) }
func (s *Store) LogsPath() string    { return filepath.Join(s.Dir(), syncLogsFile) }
func (s *Store) MigrationCheckpointPath() string {
	return filepath.Join(s.Dir(), migrationCheckpointFile)
}

func (s *Store) LoadMigrationCheckpoint() (MigrationCheckpoint, error) {
	var cp MigrationCheckpoint
	b, err := os.ReadFile(s.MigrationCheckpointPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cp, nil
		}
		return cp, err
	}
	if json.Unmarshal(b, &cp) != nil {
		return MigrationCheckpoint{}, nil
	}
	return cp, nil
}

func (s *Store) SaveMigrationCheckpoint(cp MigrationCheckpoint) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.MigrationCheckpointPath(), b, 0o644)
}

func (s *Store) LogosDir() string {
	return filepath.Join(s.Dir(), logosDirName)
}

func (s *Store) ProviderLogoPath(providerID string) string {
	id := safeProviderLogoID(providerID)
	if id == "" {
		return ""
	}
	return filepath.Join(s.LogosDir(), id+".svg")
}

func (s *Store) ensureLogosDir() error {
	return os.MkdirAll(s.LogosDir(), 0o755)
}

func (s *Store) ReadProviderLogo(providerID string) ([]byte, error) {
	path := s.ProviderLogoPath(providerID)
	if path == "" {
		return nil, os.ErrNotExist
	}
	return os.ReadFile(path)
}

func (s *Store) HasProviderLogo(providerID string) bool {
	path := s.ProviderLogoPath(providerID)
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func (s *Store) LoadPolicy() (Policy, error) {
	p := DefaultPolicy()
	b, err := os.ReadFile(s.PolicyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}
		return p, err
	}
	if json.Unmarshal(b, &p) != nil {
		return DefaultPolicy(), nil
	}
	if strings.TrimSpace(p.SourceURL) == "" {
		p.SourceURL = DefaultPolicy().SourceURL
	}
	if strings.TrimSpace(p.SyncPolicy) == "" {
		p.SyncPolicy = "scheduled"
	}
	if p.SyncIntervalHours <= 0 {
		p.SyncIntervalHours = 24
	}
	if strings.TrimSpace(p.AutoApply) == "" {
		p.AutoApply = "metadata_and_pricing"
	}
	return p, nil
}

func (s *Store) SavePolicy(p Policy) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.PolicyPath(), b, 0o644)
}

func (s *Store) LoadCatalog() (Catalog, Meta, error) {
	var cat Catalog
	var meta Meta
	b, err := os.ReadFile(s.CurrentPath())
	if err != nil {
		return nil, meta, err
	}
	if json.Unmarshal(b, &cat) != nil {
		return nil, meta, fmt.Errorf("invalid catalog json")
	}
	_ = s.loadMeta(&meta)
	return cat, meta, nil
}

func (s *Store) loadMeta(meta *Meta) error {
	b, err := os.ReadFile(s.MetaPath())
	if err != nil {
		return err
	}
	return json.Unmarshal(b, meta)
}

func (s *Store) LoadMeta() (Meta, error) {
	var meta Meta
	if err := s.loadMeta(&meta); err != nil {
		if os.IsNotExist(err) {
			return meta, nil
		}
		return meta, err
	}
	return meta, nil
}

func (s *Store) SaveCatalog(cat Catalog, meta Meta) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	body, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.CurrentPath(), body, 0o644); err != nil {
		return err
	}
	meta.Bytes = int64(len(body))
	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.MetaPath(), mb, 0o644)
}

func (s *Store) LoadRawPretty() (string, int64, error) {
	b, err := os.ReadFile(s.CurrentPath())
	if err != nil {
		return "", 0, err
	}
	var raw json.RawMessage
	if json.Unmarshal(b, &raw) != nil {
		return string(b), int64(len(b)), nil
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return string(b), int64(len(b)), nil
	}
	return string(pretty), int64(len(pretty)), nil
}

// SearchCatalogBlocks searches catalog and returns pretty-printed JSON blocks.
func (s *Store) SearchCatalogBlocks(q string, limit, offset int) ([]string, int, bool, error) {
	cat, _, err := s.LoadCatalog()
	if err != nil {
		return nil, 0, false, err
	}
	blocks, total, truncated := SearchCatalogBlocks(cat, q, limit, offset)
	return blocks, total, truncated, nil
}

// SearchRawLines is deprecated naming; returns JSON blocks in lines slice for API compat.
func (s *Store) SearchRawLines(q string, limit, offset int) ([]string, int, error) {
	blocks, total, _, err := s.SearchCatalogBlocks(q, limit, offset)
	return blocks, total, err
}

func CountCatalog(cat Catalog) (providers, models int) {
	providers = len(cat)
	for _, p := range cat {
		models += len(p.Models)
	}
	return providers, models
}
