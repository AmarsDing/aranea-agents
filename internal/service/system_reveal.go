package service

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"aranea-agents/pkg/apierror"
)

// revealLocalRequest is the POST /v1/system/reveal body (M27 Phase 5).
type revealLocalRequest struct {
	ArtifactID string `json:"artifact_id"`
}

// revealLauncher launches the OS file manager revealing the given absolute
// path. Package-level var so tests can stub OS interaction.
var revealLauncher = launchPathInFileManager

// ServeRevealLocal handles POST /v1/system/reveal. The route is registered only
// when conf.LocalRevealEnabled() (default off) — see internal/server/http.go.
func (s *ArtifactService) ServeRevealLocal(w nethttp.ResponseWriter, r *nethttp.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	var req revealLocalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRevealError(w, apierror.BadRequest(apierror.DomainArtifact, "invalid body"))
		return
	}
	abs, err := s.revealLocal(r.Context(), req.ArtifactID)
	if err != nil {
		writeRevealError(w, err)
		return
	}
	w.WriteHeader(nethttp.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true", "path": abs})
}

// revealLocal resolves the artifact's on-disk path, enforces root containment
// and launches the OS file manager. Returns the revealed absolute path.
func (s *ArtifactService) revealLocal(ctx context.Context, artifactID string) (string, error) {
	id := strings.TrimSpace(artifactID)
	if id == "" {
		return "", apierror.BadRequest(apierror.DomainArtifact, "artifact_id is required")
	}
	meta, err := s.assertWorkspaceOwnsArtifact(ctx, id, 0)
	if err != nil {
		return "", err
	}
	abs := s.uc.ResolveAbsPath(meta)
	if abs == "" {
		return "", apierror.BadRequest(apierror.DomainArtifact, "artifact is not stored locally")
	}
	// Defense-in-depth: the FS repo already enforces containment in
	// resolveBinPath; re-check here so a hostile storage_uri can never escape
	// the artifact root (M27 Phase 5 security constraint).
	if !revealPathWithinRoot(s.uc.StorageRoot(), abs) {
		return "", apierror.BadRequest(apierror.DomainArtifact, "path escapes artifact root")
	}
	if err := revealLauncher(abs); err != nil {
		return "", apierror.Internal(apierror.DomainArtifact, "launch file manager failed")
	}
	return abs, nil
}

// revealPathWithinRoot reports whether target lies within root
// (filepath.Rel anti-traversal guard). Both paths are normalized to absolute
// first: the configured storage root may be relative (e.g. "data/artifacts")
// while ResolveAbsPath always returns an absolute path — mixing the two makes
// filepath.Rel fail and would falsely reject legitimate in-root paths.
func revealPathWithinRoot(root, target string) bool {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	absRoot, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// launchPathInFileManager reveals abs in the OS file manager:
// Windows `explorer /select`, macOS `open -R`, Linux `xdg-open` on the parent
// dir. Start (not Run) detaches so the HTTP request never blocks on the file
// manager (and ignores explorer's quirky non-zero exit codes).
func launchPathInFileManager(abs string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,"+abs)
	case "darwin":
		cmd = exec.Command("open", "-R", abs)
	default:
		cmd = exec.Command("xdg-open", filepath.Dir(abs))
	}
	return cmd.Start()
}

// writeRevealError maps apierror codes to HTTP status for the custom reveal
// route (proto error encoder is not in play here).
func writeRevealError(w nethttp.ResponseWriter, err error) {
	status := nethttp.StatusInternalServerError
	msg := "internal error"
	switch {
	case apierror.IsCode(err, apierror.CodeNotFound):
		status, msg = nethttp.StatusNotFound, "artifact not found"
	case apierror.IsCode(err, apierror.CodeBadRequest):
		status = nethttp.StatusBadRequest
		if ae, ok := apierror.From(err); ok {
			msg = ae.Message
		}
	case apierror.IsCode(err, apierror.CodeForbidden):
		status, msg = nethttp.StatusForbidden, "forbidden"
	case apierror.IsCode(err, apierror.CodeUnauthorized):
		status, msg = nethttp.StatusUnauthorized, "unauthorized"
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
