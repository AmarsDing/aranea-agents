package modelregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	dirName                 = "model-catalog"
	policyFile              = "policy.json"
	currentFile             = "current.json"
	metaFile                = "current.meta.json"
	syncLogsFile            = "sync-logs.jsonl"
	migrationCheckpointFile = "migration-checkpoint.json"
	logosDirName            = "logos"
)

// Store 模型目录文件存储。LoadDirectory 走 mtime+size 失效缓存（PERF-F1：
// current.json 达 2.88MB，每请求全量读盘+反序列化导致 providers 接口 ~510ms）；
// 缓存命中时返回共享只读对象——调用方不得修改返回值（读路径均为只读组装）。
type Store struct {
	root string
	lg   loggateway.Logger

	cacheMu   sync.RWMutex
	cacheDir  Directory
	cacheMeta Meta
	cacheMod  time.Time
	cacheSize int64
	cacheOK   bool
}

func NewStore(rootDir string, lg loggateway.Logger) *Store {
	return &Store{root: strings.TrimSpace(rootDir), lg: lg}
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
		s.lg.Warn("Model registry read migration checkpoint failed", loggateway.StepID("model_registry.store.read_checkpoint_fail"), loggateway.Err(err))
		return cp, err
	}
	if err := json.Unmarshal(b, &cp); err != nil {
		s.lg.Warn("Model registry migration checkpoint file is corrupted, treating as empty",
			loggateway.StepID("model_registry.store.read_checkpoint_fail"), loggateway.Err(err))
		return MigrationCheckpoint{}, nil
	}
	return cp, nil
}

func (s *Store) SaveMigrationCheckpoint(cp MigrationCheckpoint) error {
	if err := s.ensureDir(); err != nil {
		s.lg.Warn("Model registry ensure dir for migration checkpoint failed", loggateway.StepID("model_registry.store.ensure_dir_fail"), loggateway.Err(err))
		return err
	}
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.MigrationCheckpointPath(), b, 0o644); err != nil {
		s.lg.Warn("Model registry write migration checkpoint failed", loggateway.StepID("model_registry.store.write_checkpoint_fail"), loggateway.Err(err))
		return err
	}
	return nil
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
		s.lg.Warn("Model registry read policy file failed", loggateway.StepID("model_registry.store.read_policy_fail"), loggateway.Err(err))
		return p, err
	}
	if err := json.Unmarshal(b, &p); err != nil {
		s.lg.Warn("Model registry policy file is corrupted, falling back to default policy",
			loggateway.StepID("model_registry.store.read_policy_fail"), loggateway.Err(err))
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
		s.lg.Warn("Model registry ensure dir for policy failed", loggateway.StepID("model_registry.store.ensure_dir_fail"), loggateway.Err(err))
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.PolicyPath(), b, 0o644); err != nil {
		s.lg.Warn("Model registry write policy file failed", loggateway.StepID("model_registry.store.write_policy_fail"), loggateway.Err(err))
		return err
	}
	return nil
}

// LoadDirectory 加载模型目录（mtime+size 失效缓存：os.Stat 每请求 µs 级，
// 文件未变更直接命中缓存对象；SaveDirectory 成功后主动失效双保险）。
func (s *Store) LoadDirectory() (Directory, Meta, error) {
	var meta Meta
	st, err := os.Stat(s.CurrentPath())
	if err != nil {
		s.lg.Warn("Model registry stat catalog file failed", loggateway.StepID("model_registry.store.read_catalog_fail"), loggateway.Err(err))
		return nil, meta, err
	}

	s.cacheMu.RLock()
	if s.cacheOK && s.cacheMod.Equal(st.ModTime()) && s.cacheSize == st.Size() {
		dir, m := s.cacheDir, s.cacheMeta
		s.cacheMu.RUnlock()
		return dir, m, nil
	}
	s.cacheMu.RUnlock()

	b, err := os.ReadFile(s.CurrentPath())
	if err != nil {
		s.lg.Warn("Model registry read catalog file failed", loggateway.StepID("model_registry.store.read_catalog_fail"), loggateway.Err(err))
		return nil, meta, err
	}
	var dir Directory
	if err := json.Unmarshal(b, &dir); err != nil {
		return nil, meta, fmt.Errorf("invalid catalog json: %w", err)
	}
	if err := s.loadMeta(&meta); err != nil && !os.IsNotExist(err) {
		s.lg.Warn("Model registry meta file load failed, continuing with empty meta",
			loggateway.StepID("model_registry.store.read_meta_fail"), loggateway.Err(err))
	}

	s.cacheMu.Lock()
	s.cacheDir, s.cacheMeta = dir, meta
	s.cacheMod, s.cacheSize = st.ModTime(), st.Size()
	s.cacheOK = true
	s.cacheMu.Unlock()
	return dir, meta, nil
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

func (s *Store) SaveDirectory(dir Directory, meta Meta) error {
	if err := s.ensureDir(); err != nil {
		s.lg.Warn("Model registry ensure dir for catalog failed", loggateway.StepID("model_registry.store.ensure_dir_fail"), loggateway.Err(err))
		return err
	}
	body, err := json.Marshal(dir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.CurrentPath(), body, 0o644); err != nil {
		s.lg.Warn("Model registry write catalog file failed", loggateway.StepID("model_registry.store.write_catalog_fail"), loggateway.Err(err))
		return err
	}
	meta.Bytes = int64(len(body))
	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.MetaPath(), mb, 0o644); err != nil {
		s.lg.Warn("Model registry write meta file failed", loggateway.StepID("model_registry.store.write_meta_fail"), loggateway.Err(err))
		return err
	}
	// 主动失效目录缓存（双保险：不依赖 stat 分辨率，自身写入即时可见）
	s.cacheMu.Lock()
	s.cacheOK = false
	s.cacheDir, s.cacheMeta = nil, Meta{}
	s.cacheMu.Unlock()
	return nil
}

func (s *Store) LoadRawPretty() (string, int64, error) {
	b, err := os.ReadFile(s.CurrentPath())
	if err != nil {
		return "", 0, err
	}
	var raw json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		s.lg.Warn("Model registry catalog file is not valid JSON, returning raw bytes",
			loggateway.StepID("model_registry.store.read_raw_pretty_fail"), loggateway.Err(err))
		return string(b), int64(len(b)), nil
	}
	pretty, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		s.lg.Warn("Model registry catalog pretty-print failed, returning raw bytes",
			loggateway.StepID("model_registry.store.read_raw_pretty_fail"), loggateway.Err(err))
		return string(b), int64(len(b)), nil
	}
	return string(pretty), int64(len(pretty)), nil
}

func (s *Store) SearchDirectoryBlocks(q string, limit, offset int) ([]string, int, bool, error) {
	dir, _, err := s.LoadDirectory()
	if err != nil {
		return nil, 0, false, err
	}
	blocks, total, truncated := SearchDirectoryBlocks(dir, q, limit, offset)
	return blocks, total, truncated, nil
}

func (s *Store) SearchRawLines(q string, limit, offset int) ([]string, int, error) {
	blocks, total, _, err := s.SearchDirectoryBlocks(q, limit, offset)
	return blocks, total, err
}

func CountDirectory(dir Directory) (providers, models int) {
	providers = len(dir)
	for _, p := range dir {
		models += len(p.Models)
	}
	return providers, models
}
