package modelregistry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"aranea-agents/pkg/safego"
)

const (
	defaultLogosBaseURL = "https://models.dev/logos"
	defaultLogoID       = "default"
	maxLogoBytes        = 512 << 10
	logoFetchWorkers    = 8
)

type LogoSyncResult struct {
	Synced  int
	Failed  int
	Removed int
	Errors  []string
}

func SyncProviderLogos(ctx context.Context, store *Store, cat Directory, logosBaseURL string) LogoSyncResult {
	if store == nil || len(cat) == 0 {
		return LogoSyncResult{}
	}
	base := strings.TrimRight(strings.TrimSpace(logosBaseURL), "/")
	if base == "" {
		base = defaultLogosBaseURL
	}
	if err := store.ensureLogosDir(); err != nil {
		return LogoSyncResult{Errors: []string{err.Error()}}
	}
	var res LogoSyncResult
	if err := fetchAndSaveLogo(ctx, base, store, defaultLogoID); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("logo %s: %v", defaultLogoID, err))
	}

	ids := make([]string, 0, len(cat))
	want := map[string]struct{}{}
	for id := range cat {
		safe := safeProviderLogoID(id)
		if safe == "" {
			continue
		}
		ids = append(ids, safe)
		want[safe] = struct{}{}
	}

	var mu sync.Mutex
	sem := make(chan struct{}, logoFetchWorkers)
	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		safego.Go(ctx, "modelregistry.logo_fetch", func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := fetchAndSaveLogo(ctx, base, store, id); err != nil {
				mu.Lock()
				res.Failed++
				res.Errors = append(res.Errors, fmt.Sprintf("logo %s: %v", id, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			res.Synced++
			mu.Unlock()
		})
	}
	wg.Wait()

	entries, err := os.ReadDir(store.LogosDir())
	if err == nil {
		for _, ent := range entries {
			if ent.IsDir() || filepath.Ext(ent.Name()) != ".svg" {
				continue
			}
			id := strings.TrimSuffix(ent.Name(), ".svg")
			if _, ok := want[id]; ok {
				continue
			}
			if err := os.Remove(filepath.Join(store.LogosDir(), ent.Name())); err == nil {
				res.Removed++
			}
		}
	}
	if res.Removed > 0 {
		store.invalidateLogoCache()
	}
	return res
}

func fetchAndSaveLogo(ctx context.Context, base string, store *Store, providerID string) error {
	body, err := downloadLogoSVG(ctx, base, providerID)
	if err != nil && providerID != defaultLogoID {
		body, err = downloadLogoSVG(ctx, base, defaultLogoID)
	}
	if err != nil {
		return err
	}
	tmp := store.ProviderLogoPath(providerID) + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, store.ProviderLogoPath(providerID)); err != nil {
		return err
	}
	store.invalidateLogoCache()
	return nil
}

func downloadLogoSVG(ctx context.Context, base, logoID string) ([]byte, error) {
	url := base + "/" + logoID + ".svg"
	if err := ValidateLogoSourceURL(url); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/svg+xml")
	req.Header.Set("User-Agent", "aranea-agents/model-catalog-sync")

	client := &http.Client{Timeout: defaultFetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxLogoBytes))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("empty logo")
	}
	if !looksLikeSVG(body) {
		return nil, fmt.Errorf("not svg")
	}
	return body, nil
}

func looksLikeSVG(b []byte) bool {
	s := strings.ToLower(strings.TrimSpace(string(b)))
	return strings.Contains(s, "<svg")
}

func safeProviderLogoID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return ""
	}
	return id
}

func ProviderLogoURL(providerID string) string {
	if safeProviderLogoID(providerID) == "" {
		return ""
	}
	return "/v1/model-catalog/logos/" + providerID
}
