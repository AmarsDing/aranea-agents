package deletefile

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/tools/document"
	"aranea-agents/internal/tools/editstamp"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const toolName = "delete_file"

type input struct {
	FileName string `json:"file_name,omitempty" jsonschema:"description=Workspace-relative file to delete"`
	Path     string `json:"path,omitempty" jsonschema:"description=Alias for file_name"`
}

type output struct {
	Deleted bool   `json:"deleted"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// NewTool returns delete_file for the given workspace root.
func NewTool(baseDir string, lg loggateway.Logger) trpctool.Tool {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	t := &deleter{baseDir: baseDir, lg: lg}
	return trpcfunction.NewFunctionTool(
		t.execute,
		trpcfunction.WithName(toolName),
		trpcfunction.WithDescription("Delete one file inside the workspace. Refuses directories, .git, and paths outside the workspace. Prefer this over shell rm."),
	)
}

type deleter struct {
	baseDir string
	lg      loggateway.Logger
}

func (d *deleter) execute(ctx context.Context, in input) (output, error) {
	_ = ctx
	raw := strings.TrimSpace(in.FileName)
	if raw == "" {
		raw = strings.TrimSpace(in.Path)
	}
	if raw == "" {
		return output{}, apierror.BadRequest(apierror.DomainTool, "file_name is required")
	}
	if strings.Contains(raw, "://") {
		return output{}, apierror.BadRequest(apierror.DomainTool, "delete_file does not support URI refs")
	}
	if isGitPath(raw) {
		return output{}, apierror.Forbidden(apierror.DomainTool, "refusing to delete .git paths")
	}
	candidate := raw
	if d.baseDir != "" && !filepath.IsAbs(raw) {
		candidate = filepath.Join(d.baseDir, raw)
	}
	abs, err := document.ValidatePath(candidate, d.baseDir)
	if err != nil {
		return output{}, err
	}
	if isGitPath(abs) {
		return output{}, apierror.Forbidden(apierror.DomainTool, "refusing to delete .git paths")
	}
	st, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return output{}, apierror.NotFound(apierror.DomainTool, "file not found")
		}
		return output{}, apierror.Internal(apierror.DomainTool, "stat: "+err.Error())
	}
	if st.IsDir() {
		return output{}, apierror.BadRequest(apierror.DomainTool, "refusing to delete a directory; pass a file path")
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return output{}, apierror.Forbidden(apierror.DomainTool, "refusing to delete a symlink")
	}
	if err := os.Remove(abs); err != nil {
		d.lg.Warn("delete_file failed", loggateway.StepID("tool.delete_file"), loggateway.Err(err))
		return output{}, apierror.Internal(apierror.DomainTool, "delete failed: "+err.Error())
	}
	rel := raw
	if d.baseDir != "" {
		if r, err := filepath.Rel(d.baseDir, abs); err == nil {
			rel = filepath.ToSlash(r)
		}
	} else {
		rel = filepath.ToSlash(raw)
	}
	editstamp.Record(d.baseDir, rel)
	return output{Deleted: true, Path: rel, Message: "deleted " + rel}, nil
}

func isGitPath(p string) bool {
	n := filepath.ToSlash(strings.ToLower(p))
	return n == ".git" || strings.Contains(n, "/.git/") || strings.HasPrefix(n, ".git/") || strings.HasSuffix(n, "/.git")
}
