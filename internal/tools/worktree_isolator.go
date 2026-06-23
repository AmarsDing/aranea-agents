package tools

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// WorktreeHandler executes a tool call inside a worktree directory. The
// worktreeDir is the absolute path of the freshly-created git worktree;
// implementations should perform file operations relative to it.
type WorktreeHandler func(ctx context.Context, worktreeDir string, call ToolCall) ToolResult

// WorktreeIsolator executes file-operation tool calls inside an isolated git
// worktree. On success the worktree branch is merged back to the base branch;
// on failure the worktree is discarded. This prevents concurrent file edits
// from corrupting each other.
//
// TODO(debt): P2-4 ships a minimal implementation that creates a worktree per
// call and merges via fast-forward. Production hardening (3-way merge,
// conflict resolution, branch naming strategy) is deferred until the executor
// is wired into a real tool registry.
type WorktreeIsolator struct {
	repoRoot string
	gitPath  string
	handler  WorktreeHandler
	lg       loggateway.Logger
	mu       sync.Mutex
}

// NewWorktreeIsolator creates an isolator rooted at repoRoot. The handler is
// invoked inside each worktree; if nil, Execute returns a success result with
// no side effects (useful for testing the worktree lifecycle itself).
func NewWorktreeIsolator(repoRoot string, handler WorktreeHandler, lg loggateway.Logger) (*WorktreeIsolator, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, apierror.BadRequest(apierror.DomainTool, "repoRoot is required")
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool)
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &WorktreeIsolator{
		repoRoot: absRoot,
		gitPath:  "git",
		handler:  handler,
		lg:       lg.With(loggateway.Domain(apierror.DomainTool)),
	}, nil
}

// Execute runs the tool call inside a fresh worktree. The worktree is created
// from the current HEAD, the handler is invoked with the worktree path, and
// the result determines whether the branch is merged (success) or discarded
// (failure).
func (i *WorktreeIsolator) Execute(ctx context.Context, call ToolCall) ToolResult {
	if i == nil {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: "worktree isolator not initialized"}
	}
	start := time.Now()
	result := ToolResult{CallID: call.ID, Name: call.Name}

	branchName := worktreeBranchName(call.ID)
	worktreePath, err := i.createWorktree(ctx, branchName)
	if err != nil {
		result.Error = "create worktree: " + err.Error()
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	// Ensure cleanup runs even on panic.
	defer i.removeWorktree(ctx, worktreePath, branchName)

	execResult := i.invokeHandler(ctx, worktreePath, call)
	if !execResult.Success {
		result.Error = execResult.Error
		result.Output = execResult.Output
		result.DurationMS = time.Since(start).Milliseconds()
		i.lg.Warn("worktree tool call failed, discarding branch",
			loggateway.StepID("worktree.execute"),
			loggateway.Str("call_id", call.ID),
			loggateway.Str("branch", branchName))
		return result
	}

	if err := i.mergeWorktree(ctx, branchName); err != nil {
		result.Error = "merge worktree: " + err.Error()
		result.Output = execResult.Output
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	result.Success = true
	result.Output = execResult.Output
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

// invokeHandler calls the registered handler with the worktree directory, or
// returns a default success result when no handler is configured.
func (i *WorktreeIsolator) invokeHandler(ctx context.Context, worktreeDir string, call ToolCall) ToolResult {
	if i.handler == nil {
		return ToolResult{
			CallID:  call.ID,
			Name:    call.Name,
			Success: true,
			Output:  "worktree isolated",
		}
	}
	return i.handler(ctx, worktreeDir, call)
}

// createWorktree runs `git worktree add` to create an isolated working tree
// on a new branch. Returns the absolute path of the new worktree.
func (i *WorktreeIsolator) createWorktree(ctx context.Context, branchName string) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	worktreePath := filepath.Join(i.repoRoot, ".git", "worktrees-tmp", branchName)
	args := []string{"worktree", "add", "-b", branchName, worktreePath, "HEAD"}
	if err := i.runGit(ctx, args...); err != nil {
		return "", err
	}
	return worktreePath, nil
}

// mergeWorktree merges the worktree branch back into the base branch.
// It tries fast-forward first to preserve linear history; if HEAD has
// moved due to a concurrent worktree merge, it falls back to a merge
// commit (--no-ff) so that non-conflicting parallel changes succeed.
func (i *WorktreeIsolator) mergeWorktree(ctx context.Context, branchName string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Fast path: fast-forward merge (works when no concurrent merges
	// have moved HEAD since the worktree was created).
	if err := i.runGit(ctx, "merge", "--ff-only", branchName); err == nil {
		i.deleteMergedBranch(ctx, branchName)
		return nil
	}

	// Fallback: HEAD has moved. Use --no-ff to create a merge commit.
	// This only fails on actual file conflicts, which is correct behavior.
	if err := i.runGit(ctx, "merge", "--no-ff", "-m",
		"merge parallel-exec branch "+branchName, branchName); err != nil {
		// Abort the failed merge to leave HEAD clean for the next call.
		_ = i.runGit(ctx, "merge", "--abort")
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool)
	}
	i.deleteMergedBranch(ctx, branchName)
	return nil
}

// deleteMergedBranch is best-effort cleanup after a successful merge.
// Failures are logged but do not fail the merge.
func (i *WorktreeIsolator) deleteMergedBranch(ctx context.Context, branchName string) {
	if err := i.runGit(ctx, "branch", "-d", branchName); err != nil {
		i.lg.Warn("worktree branch delete failed (post-merge)",
			loggateway.StepID("worktree.merge.cleanup"),
			loggateway.Str("branch", branchName),
			loggateway.Err(err))
	}
}

// removeWorktree prunes the worktree and deletes its branch. Errors are logged
// but not returned: this is best-effort cleanup.
func (i *WorktreeIsolator) removeWorktree(ctx context.Context, worktreePath, branchName string) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if err := i.runGit(ctx, "worktree", "remove", "--force", worktreePath); err != nil {
		i.lg.Warn("worktree removal failed",
			loggateway.StepID("worktree.cleanup"),
			loggateway.Str("path", worktreePath),
			loggateway.Err(err))
	}
	// Best-effort branch deletion; -D forces deletion of unmerged branches.
	if err := i.runGit(ctx, "branch", "-D", branchName); err != nil {
		i.lg.Warn("worktree branch delete failed (cleanup)",
			loggateway.StepID("worktree.cleanup"),
			loggateway.Str("branch", branchName),
			loggateway.Err(err))
	}
}

// runGit executes a git command in repoRoot and returns its error.
func (i *WorktreeIsolator) runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, i.gitPath, args...)
	cmd.Dir = i.repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool).
			WithMeta("git_output", string(out))
	}
	return nil
}

// worktreeBranchName builds a deterministic branch name for a call ID.
// Branch names are namespaced under "parallel-exec/" to avoid collisions.
func worktreeBranchName(callID string) string {
	// Sanitize: replace any char not in [a-zA-Z0-9._-] with '-'.
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, callID)
	return "parallel-exec/" + safe
}
