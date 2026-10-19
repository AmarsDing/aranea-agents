package artifactfs

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *FSArtifactRepo) nextVersion(dir, name string) int {

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

// StorageBytes returns total bytes stored in data files under the artifact
// root (everything except .json meta sidecars; historically only .bin).
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

			if f.IsDir() || strings.HasSuffix(f.Name(), ".json") {
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
