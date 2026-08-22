package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Compile-time check: RepoSandboxRunner implements the biz.RepoSandbox port.
var _ biz.RepoSandbox = (*RepoSandboxRunner)(nil)

const (
	// defaultWorktreeRoot isolates self-improvement worktrees inside the repo
	// (design D4; must stay gitignored).
	defaultWorktreeRoot = ".aranea-self-improve"
	// maxGateOutputBytes caps gate output captured into the verification
	// report (design D4: 64KB).
	maxGateOutputBytes = 64 * 1024
	// sandboxCleanupTimeout bounds detached cleanup git commands so a hung
	// git process cannot leak worktrees on shutdown.
	sandboxCleanupTimeout = 30 * time.Second
)

// defaultGateTimeouts follows design D4 (G1 5m / G2 10m / G3 5m).
var defaultGateTimeouts = map[biz.SandboxGateKind]time.Duration{
	biz.SandboxGateBuild: 5 * time.Minute,
	biz.SandboxGateTest:  10 * time.Minute,
	biz.SandboxGateLint:  5 * time.Minute,
}

// RepoSandboxOption customizes RepoSandboxRunner.
type RepoSandboxOption func(*RepoSandboxRunner)

// WithWorktreeRoot overrides the worktree root directory (relative to repo
// root). Empty values are ignored.
func WithWorktreeRoot(root string) RepoSandboxOption {
	return func(r *RepoSandboxRunner) {
		if strings.TrimSpace(root) != "" {
			r.worktreeRoot = root
		}
	}
}

// WithGolangCILint enables package-level golangci-lint for G3 when the
// worktree has a config and the binary is on PATH. Default is go vet so
// historical lint debt cannot re-block the code channel.
func WithGolangCILint(enabled bool) RepoSandboxOption {
	return func(r *RepoSandboxRunner) {
		r.golangciEnabled = enabled
	}
}

// RepoRoot returns the absolute repository root the sandbox is anchored at.
func (r *RepoSandboxRunner) RepoRoot() string {
	if r == nil {
		return ""
	}
	return r.repoRoot
}

// WithGateTimeout overrides one gate's execution timeout. d <= 0 is ignored.
func WithGateTimeout(gate biz.SandboxGateKind, d time.Duration) RepoSandboxOption {
	return func(r *RepoSandboxRunner) {
		if d > 0 {
			r.gateTimeouts[gate] = d
		}
	}
}

// RepoSandboxRunner runs the self-improvement verification gates G1-G3 inside
// a dedicated git worktree (design D4). Gates execute fixed whitelisted
// commands (go build / go test / go vet) — gate input never becomes shell
// text.
type RepoSandboxRunner struct {
	repoRoot        string
	worktreeRoot    string
	gateTimeouts    map[biz.SandboxGateKind]time.Duration
	golangciEnabled bool
	lg              loggateway.Logger
	mu              sync.Mutex // serialize git worktree add/remove (project convention)
}

// NewRepoSandboxRunner creates a runner rooted at the repository work dir.
func NewRepoSandboxRunner(repoRoot string, lg loggateway.Logger, opts ...RepoSandboxOption) (*RepoSandboxRunner, error) {
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
	r := &RepoSandboxRunner{
		repoRoot:     absRoot,
		worktreeRoot: defaultWorktreeRoot,
		gateTimeouts: map[biz.SandboxGateKind]time.Duration{},
		lg:           lg.With(loggateway.Domain(apierror.DomainTool)),
	}
	for k, v := range defaultGateTimeouts {
		r.gateTimeouts[k] = v
	}
	for _, opt := range opts {
		opt(r)
	}
	return r, nil
}

// PrepareWorktree creates worktree <repoRoot>/<worktreeRoot>/<runID> on
// branch self-improve/<runID> based at baseRef (empty = HEAD). The returned
// cleanup removes worktree + branch; it is idempotent and uses a detached
// context so it still runs when the caller's ctx is cancelled.
func (r *RepoSandboxRunner) PrepareWorktree(ctx context.Context, runID, baseRef string) (string, func(), error) {
	safe := sanitizeSandboxRunID(runID)
	if safe == "" {
		return "", nil, apierror.BadRequest(apierror.DomainTool, "runID is required")
	}
	branch := "self-improve/" + safe
	wtPath := filepath.Join(r.repoRoot, r.worktreeRoot, safe)
	ref := strings.TrimSpace(baseRef)
	if ref == "" {
		ref = "HEAD"
	}

	r.mu.Lock()
	// 2026-08-19 事故：git 在注册 worktree 后、checkout 完成前被杀（exit 255），
	// 留下"已注册的半截 worktree"。先快照注册态以区分两种已注册报错：
	// pre-registered = 活跃重复 runID（不动、直接报错）；post-only = 本次调用半截
	// 创建（归我们清理，否则 worktree+分支永久泄漏，run 已失败无人持有路径）。
	preRegistered := r.worktreeRegistered(ctx, wtPath)
	err := r.runGit(ctx, r.repoRoot, nil, "worktree", "add", "-b", branch, wtPath, ref)
	if err != nil && !preRegistered && r.worktreeRegistered(ctx, wtPath) {
		r.purgeRegisteredWorktree(ctx, branch, wtPath)
	} else if err != nil && !r.worktreeRegistered(ctx, wtPath) {
		// 崩溃孤儿自愈（2026-08-08 exit 128 事故）：分支已建/目录残留但 worktree
		// 未注册 = 上次 prepare 中途失败（或进程被杀）的遗留。清理后重试一次；
		// 再失败同样清理，保证错误返回后无孤儿，下次 stale 恢复仍是干净起点。
		// 已注册的同名 worktree（活跃重复 runID）不在此清理，直接报错。
		r.purgeWorktreeOrphan(ctx, branch, wtPath)
		if retryErr := r.runGit(ctx, r.repoRoot, nil, "worktree", "add", "-b", branch, wtPath, ref); retryErr != nil {
			if r.worktreeRegistered(ctx, wtPath) {
				r.purgeRegisteredWorktree(ctx, branch, wtPath)
			} else {
				r.purgeWorktreeOrphan(ctx, branch, wtPath)
			}
			r.mu.Unlock()
			return "", nil, err
		}
		err = nil
	}
	r.mu.Unlock()
	if err != nil {
		return "", nil, err
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxCleanupTimeout)
			defer cancel()
			r.mu.Lock()
			defer r.mu.Unlock()
			if err := r.runGit(cleanupCtx, r.repoRoot, nil, "worktree", "remove", "--force", wtPath); err != nil {
				r.lg.Warn("self-improve worktree removal failed",
					loggateway.StepID("sandbox.worktree.cleanup"),
					loggateway.Str("path", wtPath),
					loggateway.Err(err))
			}
			if err := r.runGit(cleanupCtx, r.repoRoot, nil, "branch", "-D", branch); err != nil {
				r.lg.Warn("self-improve branch delete failed",
					loggateway.StepID("sandbox.worktree.cleanup"),
					loggateway.Str("branch", branch),
					loggateway.Err(err))
			}
		})
	}
	return wtPath, cleanup, nil
}

// removeWorktreeKeepBranch drops the worktree directory but leaves the
// self-improve/<runID> branch so ApplyCodeMerge can persist a reviewable
// commit without mutating the current HEAD.
func (r *RepoSandboxRunner) removeWorktreeKeepBranch(ctx context.Context, wtPath string) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sandboxCleanupTimeout)
	defer cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.runGit(cleanupCtx, r.repoRoot, nil, "worktree", "remove", "--force", wtPath); err != nil {
		r.lg.Warn("self-improve worktree removal (keep branch) failed",
			loggateway.StepID("sandbox.worktree.keep_branch"),
			loggateway.Str("path", wtPath),
			loggateway.Err(err))
	}
}

// ApplyDiff applies a unified diff inside the worktree via `git apply`.
// The diff travels over stdin, never through shell interpolation.
func (r *RepoSandboxRunner) ApplyDiff(ctx context.Context, path, diff string) error {
	if strings.TrimSpace(diff) == "" {
		return apierror.BadRequest(apierror.DomainTool, "diff is empty")
	}
	wtPath, err := r.checkSandboxPath(path)
	if err != nil {
		return err
	}
	return r.runGit(ctx, wtPath, strings.NewReader(diff), "apply", "--whitespace=nowarn", "-")
}

// ResetWorktree restores the worktree to its base-ref state (git reset --hard
// + clean -fd), discarding any previously applied diff. The pipeline calls it
// before every Patcher attempt (biz.RepoSandbox port doc).
func (r *RepoSandboxRunner) ResetWorktree(ctx context.Context, path string) error {
	wtPath, err := r.checkSandboxPath(path)
	if err != nil {
		return err
	}
	if err := r.runGit(ctx, wtPath, nil, "reset", "--hard"); err != nil {
		return err
	}
	return r.runGit(ctx, wtPath, nil, "clean", "-fd")
}

// RunGate executes one verification gate inside the worktree. G4 (Critic) and
// G5 (Eval baseline) are not runner gates and return an explicit error.
func (r *RepoSandboxRunner) RunGate(ctx context.Context, path string, gate biz.SandboxGateKind, pkgs []string) (biz.SandboxGateResult, error) {
	wtPath, err := r.checkSandboxPath(path)
	if err != nil {
		return biz.SandboxGateResult{}, err
	}
	if (gate == biz.SandboxGateTest || gate == biz.SandboxGateLint) && len(pkgs) == 0 {
		return biz.NewSkippedSandboxGate(gate, "skipped: no affected Go packages (refusing repo-wide ./...)"), nil
	}
	if gate == biz.SandboxGateWebLint {
		return r.runWebLint(ctx, wtPath)
	}

	name, args, err := r.gateCommand(wtPath, gate, pkgs)
	if err != nil {
		return biz.SandboxGateResult{}, err
	}
	res := r.execGate(ctx, wtPath, gate, name, args)
	if !res.Passed {
		r.lg.Warn("self-improve sandbox gate failed",
			loggateway.StepID("sandbox.gate"),
			loggateway.Str("gate", string(gate)),
			loggateway.Str("worktree", wtPath))
	}
	return res, nil
}

// ProbeTestFailures runs the G2 command on the current worktree and returns
// named failing tests. Compile/setup failures return an empty list.
func (r *RepoSandboxRunner) ProbeTestFailures(ctx context.Context, path string, pkgs []string) ([]string, error) {
	if len(pkgs) == 0 {
		return nil, nil
	}
	wtPath, err := r.checkSandboxPath(path)
	if err != nil {
		return nil, err
	}
	name, args, err := r.gateCommand(wtPath, biz.SandboxGateTest, pkgs)
	if err != nil {
		return nil, err
	}
	res := r.execGate(ctx, wtPath, biz.SandboxGateTest, name, args)
	names, setup := biz.ParseGoTestFailures(res.Output)
	if setup {
		return nil, nil
	}
	return names, nil
}

func (r *RepoSandboxRunner) execGate(ctx context.Context, wtPath string, gate biz.SandboxGateKind, name string, args []string) biz.SandboxGateResult {
	timeout := r.gateTimeouts[gate]
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	gateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(gateCtx, name, args...)
	cmd.Dir = wtPath
	cmd.Env = sandboxGateEnv()
	out, runErr := cmd.CombinedOutput()
	res := biz.SandboxGateResult{
		Gate:       gate,
		Passed:     runErr == nil,
		Output:     truncateGateOutput(string(out)),
		DurationMS: time.Since(start).Milliseconds(),
	}
	if runErr != nil {
		if gateCtx.Err() == context.DeadlineExceeded {
			res.Output = appendGateOutputNote(res.Output, fmt.Sprintf("[gate timeout after %s]", timeout))
		} else {
			res.Output = appendGateOutputNote(res.Output, "[exit "+exitCodeString(runErr)+"]")
		}
	}
	return res
}

// gateCommand maps a gate kind to its whitelisted command. Package patterns
// are passed as discrete argv entries (no shell). Empty pkgs must not reach
// G2/G3 (RunGate returns Skipped first).
func (r *RepoSandboxRunner) gateCommand(wtPath string, gate biz.SandboxGateKind, pkgs []string) (string, []string, error) {
	switch gate {
	case biz.SandboxGateBuild:
		// Design D4: G1 always builds repo-wide — a patch can break packages
		// outside its own scope via shared symbols.
		return "go", []string{"build", "./..."}, nil
	case biz.SandboxGateTest:
		return "go", append([]string{"test", "-count=1"}, pkgs...), nil
	case biz.SandboxGateLint:
		if name, args, ok := r.golangciLintCommand(wtPath, pkgs); ok {
			return name, args, nil
		}
		return "go", append([]string{"vet"}, pkgs...), nil
	default:
		return "", nil, apierror.BadRequest(apierror.DomainTool,
			"gate %s is not executed by RepoSandboxRunner (G4=Critic agent, G5=eval baseline)", gate)
	}
}

func (r *RepoSandboxRunner) golangciLintCommand(wtPath string, pkgs []string) (string, []string, bool) {
	if r == nil || (!r.golangciEnabled && os.Getenv("ARANEA_SI_GOLANGCI") != "1") {
		return "", nil, false
	}
	if !hasGolangCIConfig(wtPath) {
		return "", nil, false
	}
	lint, err := exec.LookPath("golangci-lint")
	if err != nil {
		return "", nil, false
	}
	return lint, append([]string{"run", "--timeout", "4m"}, pkgs...), true
}

func hasGolangCIConfig(root string) bool {
	for _, name := range []string{".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json"} {
		if st, err := os.Stat(filepath.Join(root, name)); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func (r *RepoSandboxRunner) runWebLint(ctx context.Context, wtPath string) (biz.SandboxGateResult, error) {
	webDir := filepath.Join(wtPath, "web")
	if st, err := os.Stat(webDir); err != nil || !st.IsDir() {
		return biz.NewSkippedSandboxGate(biz.SandboxGateWebLint, "skipped: web/ not present in worktree"), nil
	}
	if _, err := exec.LookPath("pnpm"); err != nil {
		return biz.NewSkippedSandboxGate(biz.SandboxGateWebLint, "skipped: pnpm not on PATH"), nil
	}
	timeout := r.gateTimeouts[biz.SandboxGateLint]
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	gateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	cmd := exec.CommandContext(gateCtx, "pnpm", "lint")
	cmd.Dir = webDir
	cmd.Env = sandboxGateEnv()
	out, runErr := cmd.CombinedOutput()
	res := biz.SandboxGateResult{
		Gate:       biz.SandboxGateWebLint,
		Passed:     runErr == nil,
		Output:     truncateGateOutput(string(out)),
		DurationMS: time.Since(start).Milliseconds(),
	}
	if runErr != nil {
		if gateCtx.Err() == context.DeadlineExceeded {
			res.Output = appendGateOutputNote(res.Output, fmt.Sprintf("[gate timeout after %s]", timeout))
		} else {
			res.Output = appendGateOutputNote(res.Output, "[exit "+exitCodeString(runErr)+"]")
		}
	}
	return res, nil
}

// sandboxGateEnv is the allowlisted environment for gate subprocesses.
// Production DSNs / secrets are stripped so G2 cannot reach the live database.
func sandboxGateEnv() []string {
	out := []string{"ARANEA_SI_SANDBOX=1", "GIT_CONFIG_NOSYSTEM=1"}
	for _, kv := range os.Environ() {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if !sandboxEnvAllowed(key, val) {
			continue
		}
		out = append(out, key+"="+val)
	}
	return out
}

func sandboxEnvAllowed(key, val string) bool {
	uk := strings.ToUpper(key)
	if _, blocked := sandboxEnvBlockExact[uk]; blocked {
		return false
	}
	if strings.Contains(uk, "PASSWORD") || strings.Contains(uk, "SECRET") ||
		strings.Contains(uk, "API_KEY") || strings.Contains(uk, "ACCESS_TOKEN") ||
		strings.HasSuffix(uk, "_DSN") || uk == "DSN" || strings.Contains(uk, "DATABASE") {
		return false
	}
	lv := strings.ToLower(val)
	if strings.Contains(lv, "postgres://") || strings.Contains(lv, "postgresql://") ||
		strings.Contains(lv, "mysql://") || strings.Contains(lv, "redis://") ||
		strings.Contains(lv, "mongodb://") {
		return false
	}
	_, allow := sandboxEnvAllow[uk]
	return allow
}

var sandboxEnvAllow = map[string]struct{}{
	"PATH": {}, "PATHEXT": {}, "SYSTEMROOT": {}, "WINDIR": {}, "COMSPEC": {},
	"TMP": {}, "TEMP": {}, "TMPDIR": {},
	"GOROOT": {}, "GOPATH": {}, "GOBIN": {}, "GOTOOLCHAIN": {},
	"GOCACHE": {}, "GOMODCACHE": {}, "GOFLAGS": {}, "GO111MODULE": {},
	"GOPROXY": {}, "GOSUMDB": {}, "GONOSUMDB": {}, "GOPRIVATE": {}, "GOINSECURE": {},
	"CGO_ENABLED": {}, "CC": {}, "CXX": {}, "CGO_CFLAGS": {}, "CGO_LDFLAGS": {},
	"HOME": {}, "USERPROFILE": {}, "HOMEDRIVE": {}, "HOMEPATH": {},
	"USER": {}, "USERNAME": {}, "LOGNAME": {},
	"APPDATA": {}, "LOCALAPPDATA": {},
	"LANG": {}, "LC_ALL": {}, "TZ": {},
}

var sandboxEnvBlockExact = map[string]struct{}{
	"DATABASE_URL": {}, "PGHOST": {}, "PGPORT": {}, "PGUSER": {}, "PGPASSWORD": {},
	"PGDATABASE": {}, "PGDATA": {}, "PGSSLMODE": {},
	"REDIS_URL": {}, "REDIS_HOST": {},
}

// checkSandboxPath resolves path and ensures it lives under the configured
// worktree root, blocking gate/apply calls against the main repo or arbitrary
// directories.
func (r *RepoSandboxRunner) checkSandboxPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", apierror.BadRequest(apierror.DomainTool, "worktree path is required")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool)
	}
	sandboxRoot := filepath.Join(r.repoRoot, r.worktreeRoot)
	if abs != sandboxRoot && !strings.HasPrefix(abs, sandboxRoot+string(os.PathSeparator)) {
		return "", apierror.BadRequest(apierror.DomainTool,
			"path %q is outside the sandbox worktree root %q", p, sandboxRoot)
	}
	return abs, nil
}

// worktreeRegistered reports whether wtPath is a currently registered git
// worktree. Conservative: returns true when the listing itself fails, so an
// active worktree is never mistaken for a crash orphan and purged.
func (r *RepoSandboxRunner) worktreeRegistered(ctx context.Context, wtPath string) bool {
	cmd := exec.CommandContext(ctx, "git", "-c", "core.longpaths=true", "worktree", "list", "--porcelain")
	cmd.Dir = r.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return true
	}
	target := filepath.ToSlash(wtPath)
	for line := range strings.Lines(string(out)) {
		if p, ok := strings.CutPrefix(line, "worktree "); ok {
			if strings.EqualFold(strings.TrimRight(p, "\r\n"), target) {
				return true
			}
		}
	}
	return false
}

// purgeWorktreeOrphan best-effort removes a crash-orphaned branch + leftover
// directory (no registered worktree). Errors are logged, never fatal: the
// subsequent add retry is the real verdict.
func (r *RepoSandboxRunner) purgeWorktreeOrphan(ctx context.Context, branch, wtPath string) {
	if err := r.runGit(ctx, r.repoRoot, nil, "branch", "-D", branch); err != nil {
		r.lg.Debug("self-improve orphan branch delete skipped",
			loggateway.StepID("sandbox.worktree.purge"),
			loggateway.Str("branch", branch),
			loggateway.Err(err))
	}
	if err := os.RemoveAll(wtPath); err != nil {
		r.lg.Warn("self-improve orphan dir removal failed",
			loggateway.StepID("sandbox.worktree.purge"),
			loggateway.Str("path", wtPath),
			loggateway.Err(err))
	}
}

// purgeRegisteredWorktree removes a half-created worktree that IS registered
// (git died after registration, e.g. 2026-08-19 exit 255): worktree remove
// unregisters + deletes the dir, then the branch is dropped. Best-effort like
// purgeWorktreeOrphan; the error return to the caller is the real verdict.
func (r *RepoSandboxRunner) purgeRegisteredWorktree(ctx context.Context, branch, wtPath string) {
	if err := r.runGit(ctx, r.repoRoot, nil, "worktree", "remove", "--force", wtPath); err != nil {
		r.lg.Warn("self-improve half-created worktree removal failed",
			loggateway.StepID("sandbox.worktree.purge"),
			loggateway.Str("path", wtPath),
			loggateway.Err(err))
	}
	r.purgeWorktreeOrphan(ctx, branch, wtPath)
}

// runGit executes a git command with optional stdin and wraps failures with
// the captured output for diagnostics.
func (r *RepoSandboxRunner) runGit(ctx context.Context, dir string, stdin *strings.Reader, args ...string) error {
	// core.longpaths：Windows 下沙盒路径前缀（repoRoot + .aranea-self-improve +
	// uuid ≈ 76 字符）叠加仓库长路径文件（bench 输出最长达 229 字符）撞
	// MAX_PATH(260)，checkout/cleanup 均 "Filename too long" exit 128
	//（2026-08-08 事故）；非 Windows 平台该配置无害。
	args = append([]string{"-c", "core.longpaths=true"}, args...)
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	if stdin != nil {
		cmd.Stdin = stdin
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainTool).
			WithMeta("git_args", strings.Join(args, " ")).
			WithMeta("git_output", truncateGateOutput(string(out)))
	}
	return nil
}

// sanitizeSandboxRunID maps a run ID to a safe path/branch component. Dots
// are folded too: ".." is illegal inside git ref names, and run IDs are
// UUIDs that never carry meaningful dots. Consecutive separators collapse
// into one so hostile input cannot smuggle ref sequences or path traversal.
func sanitizeSandboxRunID(runID string) string {
	mapped := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, strings.TrimSpace(runID))
	for strings.Contains(mapped, "--") {
		mapped = strings.ReplaceAll(mapped, "--", "-")
	}
	return strings.Trim(mapped, "-")
}

// truncateGateOutput keeps the head of gate output within the 64KB report
// budget, appending a truncation marker.
func truncateGateOutput(out string) string {
	if len(out) <= maxGateOutputBytes {
		return out
	}
	return out[:maxGateOutputBytes] + "\n...[truncated]"
}

// appendGateOutputNote appends a status note within the output budget.
func appendGateOutputNote(out, note string) string {
	if len(out)+len(note)+1 > maxGateOutputBytes+64 {
		out = truncateGateOutput(out)
	}
	if out == "" {
		return note
	}
	return out + "\n" + note
}

// exitCodeString extracts a human-readable exit description.
func exitCodeString(err error) string {
	if exitErr, ok := err.(*exec.ExitError); ok {
		return fmt.Sprintf("%d", exitErr.ExitCode())
	}
	return "error: " + err.Error()
}
