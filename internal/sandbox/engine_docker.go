package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Container label keys (the only source of truth, design §4.2).
const (
	LabelSandbox  = "aranea.sandbox"
	LabelID       = "aranea.sandbox.id"
	LabelProfile  = "aranea.sandbox.profile"
	LabelAgentKey = "aranea.sandbox.agent_key"
	LabelSession  = "aranea.sandbox.session_id"
	LabelRun      = "aranea.sandbox.run_id"
	LabelDeadline = "aranea.sandbox.deadline"
)

// DockerEngine implements Engine over the docker CLI, reusing the daemon
// reachability model of M32 (docker.sock passthrough; CLI works regardless
// of where the daemon lives). Instances are long-running containers with a
// `sleep infinity` entrypoint; commands run via `docker exec`.
type DockerEngine struct {
	// run is the process runner hook (tests stub it).
	run func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewDockerEngine returns a DockerEngine backed by the real docker CLI.
func NewDockerEngine() *DockerEngine {
	return &DockerEngine{run: exec.CommandContext}
}

func (e *DockerEngine) cmd(ctx context.Context, args ...string) *exec.Cmd {
	run := e.run
	if run == nil {
		run = exec.CommandContext
	}
	return run(ctx, "docker", args...)
}

// dockerNetworkFor maps the profile network stance onto a docker network
// argument (P2-1): none/full are daemon built-ins; egress attaches to the
// no-masquerade lane bridge resolved in normalize().
func dockerNetworkFor(p Profile) string {
	switch p.Network {
	case NetworkFull:
		return "bridge"
	case NetworkEgress:
		if p.EgressNetwork != "" {
			return p.EgressNetwork
		}
		return "aranea-egress"
	default:
		return "none"
	}
}

// Create assembles `docker create` from the M32 isolation baseline
// (--network/--memory/--cpus/--read-only/tmpfs) plus the M82 hardening
// additions (cap-drop ALL, no-new-privileges, pids-limit), then starts the
// instance so it is ready to accept exec.
func (e *DockerEngine) Create(ctx context.Context, sandboxID string, p Profile, labels map[string]string) (Handle, error) {
	p = p.withDefaults()
	h := Handle{ID: sandboxID, SandboxID: sandboxID}

	args := []string{
		"create",
		"--name", h.ID,
		"--network", dockerNetworkFor(p),
		fmt.Sprintf("--memory=%d", p.MemoryBytes),
		fmt.Sprintf("--memory-swap=%d", p.MemoryBytes), // disable swap
		fmt.Sprintf("--cpus=%g", p.CPUs),
		fmt.Sprintf("--pids-limit=%d", p.PidsLimit),
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges",
		"--stop-timeout=5",
		// Anonymous volume for artifact output: writable under --read-only and
		// preserved for `docker cp` collection; removed by rm -v. Path matches
		// the M32 agent-facing contract (/workspace/out).
		"--volume", "/workspace/out",
	}
	// P2-1 egress lane: the bridge has no NAT masquerade, so the only way out
	// is the CONNECT proxy (injected via env; upper+lowercase variants cover
	// curl/python/node/git conventions). Domain whitelist enforced proxy-side.
	if p.Network == NetworkEgress && p.EgressProxy != "" {
		args = append(args,
			"--env", "HTTP_PROXY="+p.EgressProxy,
			"--env", "HTTPS_PROXY="+p.EgressProxy,
			"--env", "http_proxy="+p.EgressProxy,
			"--env", "https_proxy="+p.EgressProxy,
			"--env", "NO_PROXY=localhost,127.0.0.1",
			"--env", "no_proxy=localhost,127.0.0.1",
		)
	}
	if p.ReadOnlyRootfs {
		args = append(args, "--read-only", "--tmpfs", "/tmp:size="+p.TmpSize)
	}
	for k, v := range labels {
		args = append(args, "--label", k+"="+v)
	}
	// sh exists in virtually every image; --entrypoint beats image CMD/ENTRYPOINT.
	args = append(args, "--entrypoint", "sh", p.Image, "-c", "sleep infinity")

	if out, err := e.cmd(ctx, args...).CombinedOutput(); err != nil {
		return Handle{}, fmt.Errorf("sandbox docker create %s: %w (%s)", h.ID, err, strings.TrimSpace(string(out)))
	}
	if out, err := e.cmd(ctx, "start", h.ID).CombinedOutput(); err != nil {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = e.cmd(cleanCtx, "rm", "-fv", h.ID).Run()
		return Handle{}, fmt.Errorf("sandbox docker start %s: %w (%s)", h.ID, err, strings.TrimSpace(string(out)))
	}
	return h, nil
}

// Exec runs the command via `docker exec -i` with stdin streaming. On ctx
// deadline the local CLI is killed; any orphaned in-container process is
// bounded by the use-and-destroy contract (the instance is torn down at
// Release/TTL). Exit 124 is also treated as a timeout (in-container
// `timeout` wrapper, when a consumer uses one); 137 as OOM (M32 parity).
func (e *DockerEngine) Exec(ctx context.Context, h Handle, spec ExecSpec) (ExecResult, error) {
	if len(spec.Argv) == 0 {
		return ExecResult{}, fmt.Errorf("sandbox exec: empty argv")
	}
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append([]string{"exec", "-i", h.ID}, spec.Argv...)
	cmd := e.cmd(ctx, args...)
	cmd.Stdin = strings.NewReader(spec.Stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	switch {
	case ctx.Err() == context.DeadlineExceeded || res.ExitCode == 124:
		res.TimedOut = true
	case res.ExitCode == 137:
		res.OOM = true
	}
	// A non-zero exit (or killed CLI) is reported in-band, never as a Go error,
	// matching M32 semantics: Go errors mean the sandbox itself is broken.
	if err != nil && res.ExitCode == 0 && !res.TimedOut {
		return res, fmt.Errorf("sandbox docker exec %s: %w", h.ID, err)
	}
	return res, nil
}

// CopyTo streams a tar archive from r into the instance at path (`docker cp -`).
func (e *DockerEngine) CopyTo(ctx context.Context, h Handle, path string, r io.Reader) error {
	cmd := e.cmd(ctx, "cp", "-", h.ID+":"+path)
	cmd.Stdin = r
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sandbox docker cp to %s:%s: %w (%s)", h.ID, path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CopyFrom reads path from the instance as a tar stream (`docker cp ... -`).
func (e *DockerEngine) CopyFrom(ctx context.Context, h Handle, path string) (io.ReadCloser, error) {
	cmd := e.cmd(ctx, "cp", h.ID+":"+path, "-")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("sandbox docker cp from %s:%s: %w", h.ID, path, err)
	}
	return &cmdReadCloser{ReadCloser: out, cmd: cmd}, nil
}

// Destroy force-removes the instance and its anonymous volumes. Errors are
// swallowed: destroy is idempotent and "already gone" is indistinguishable
// from other daemon errors without extra round-trips.
func (e *DockerEngine) Destroy(ctx context.Context, h Handle) error {
	_ = e.cmd(ctx, "rm", "-fv", h.ID).Run()
	return nil
}

// ListByLabels returns all instances (any state) carrying every given label.
func (e *DockerEngine) ListByLabels(ctx context.Context, labels map[string]string) ([]Handle, error) {
	args := []string{"ps", "-a", "--format", "{{.Names}}"}
	for k, v := range labels {
		args = append(args, "--filter", "label="+k+"="+v)
	}
	out, err := e.cmd(ctx, args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sandbox docker ps: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	var hs []Handle
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		hs = append(hs, Handle{ID: name, SandboxID: name})
	}
	return hs, nil
}

// cmdReadCloser closes the stdout pipe and waits for the CLI to exit.
type cmdReadCloser struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (c *cmdReadCloser) Close() error {
	_ = c.ReadCloser.Close()
	return c.cmd.Wait()
}

// --- daemon probe (30s TTL cache, mirrors codeexecutor/docker_probe.go) ---

const dockerProbeTTL = 30 * time.Second

var (
	dockerProbeMu  sync.Mutex
	dockerProbeOK  bool
	dockerProbeAt  time.Time
	dockerProbeRun = func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").Run() == nil
	}
	dockerNow = time.Now
)

// DockerDaemonAvailable reports whether the docker daemon responds, cached
// for dockerProbeTTL so a daemon started later becomes visible without restart.
func DockerDaemonAvailable() bool {
	dockerProbeMu.Lock()
	defer dockerProbeMu.Unlock()
	if !dockerProbeAt.IsZero() && dockerNow().Sub(dockerProbeAt) < dockerProbeTTL {
		return dockerProbeOK
	}
	dockerProbeOK = dockerProbeRun()
	dockerProbeAt = dockerNow()
	return dockerProbeOK
}

// ResetDockerDaemonProbe clears the cached probe result (tests only).
func ResetDockerDaemonProbe() {
	dockerProbeMu.Lock()
	defer dockerProbeMu.Unlock()
	dockerProbeAt = time.Time{}
	dockerProbeOK = false
}
