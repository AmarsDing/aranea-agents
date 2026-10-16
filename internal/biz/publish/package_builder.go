package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildPackage writes a mode-A release package directory + manifest.json.
// Success means status=packaged (human still pastes into platform UI).
func BuildPackage(rootDir string, job Job) (manifestPath string, manifest PackageManifest, err error) {
	if strings.TrimSpace(job.Title) == "" {
		return "", PackageManifest{}, fmt.Errorf("publish: title required")
	}
	dir := filepath.Join(rootDir, job.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", PackageManifest{}, err
	}
	bodyPath := filepath.Join(dir, "body.md")
	if err := os.WriteFile(bodyPath, []byte(job.Body), 0o644); err != nil {
		return "", PackageManifest{}, err
	}
	m := PackageManifest{
		SchemaVersion:   "1.0",
		Platform:        job.Platform,
		Mode:            ModePackage,
		Title:           job.Title,
		Body:            job.Body,
		Tags:            job.Tags,
		MediaPaths:      job.MediaPaths,
		UTM:             map[string]string{"source": "aranea", "medium": "package"},
		ComplianceNote:  "Human must paste/upload; not auto-posted to platform.",
		ReviewTaskID:    job.ReviewTaskID,
		ContentItemID:   job.ContentItemID,
		StatusSemantics: "packaged_not_posted_to_platform",
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", PackageManifest{}, err
	}
	manifestPath = filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(manifestPath, b, 0o644); err != nil {
		return "", PackageManifest{}, err
	}
	return manifestPath, m, nil
}
