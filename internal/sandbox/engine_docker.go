package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
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
// argument for none/full. Egress is handled separately in Create: each
// egress sandbox gets a DEDICATED internal network (egressNetName) shared
// only with the proxy — a shared egress bridge would leave all sandboxes
// mutually reachable (Docker user-defined bridges default ICC on), breaking
// inter-sandbox isolation (review 2026-08-26 #3).
func dockerNetworkFor(p Profile) string {
	switch p.Network {
	case NetworkFull:
		return "bridge"
	default:
		return "none"
	}
}

// egressNetName derives the per-sandbox egress network name: the configured
// lane prefix (default "aranea-egress") + the sandbox id.
func egressNetName(p Profile, sandboxID string) string {
	prefix := p.EgressNetwork
	if prefix == "" {
		prefix = "aranea-egress"
	}
	return prefix + "-" + sandboxID
}

// egressProxyHost extracts the proxy container name from the proxy URL
// (embedded DNS resolves connected containers by name).
func egressProxyHost(proxyURL string) string {
	if u, err := url.Parse(proxyURL); err == nil && u.Hostname() != "" {
		return u.Hostname()
	}
	return "aranea-egress-proxy"
}

// createEgressNetwork provisions the per-sandbox internal network and
// connects the egress proxy to it (the sandbox's only reachable peer and
// only way out). Labels mirror the container labels so the startup reconcile
// can sweep orphans (NetworkReaper).
func (e *DockerEngine) createEgressNetwork(ctx context.Context, netName, sandboxID, proxyHost string) error {
	args := []string{
		"network", "create", "--internal",
		"--label", LabelSandbox + "=1",
		"--label", LabelID + "=" + sandboxID,
		netName,
	}
	if out, err := e.cmd(ctx, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("sandbox egress network create %s: %w (%s)", netName, err, strings.TrimSpace(string(out)))
	}
	if out, err := e.cmd(ctx, "network", "connect", netName, proxyHost).CombinedOutput(); err != nil {
		_ = e.removeEgressNetwork(ctx, netName)
		return fmt.Errorf("sandbox egress network connect proxy %s to %s: %w (%s)", proxyHost, netName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// removeEgressNetwork force-disconnects remaining endpoints (the long-lived
// proxy otherwise keeps the network non-removable) and deletes the network.
// Silent-success when the network does not exist (non-egress sandboxes share
// the Destroy path); other daemon errors are joined and returned (review
// 2026-08-26 r2 #4 — teardown failures must not be invisible).
func (e *DockerEngine) removeEgressNetwork(ctx context.Context, netName string) error {
	out, err := e.cmd(ctx, "network", "inspect", "-f", "{{range $id, $_ := .Containers}}{{$id}} {{end}}", netName).CombinedOutput()
	if err != nil {
		return nil // network absent or daemon error on lookup — nothing to do
	}
	var errs []error
	for _, id := range strings.Fields(string(out)) {
		if out2, err2 := e.cmd(ctx, "network", "disconnect", "-f", netName, id).CombinedOutput(); err2 != nil {
			errs = append(errs, fmt.Errorf("sandbox egress network disconnect %s from %s: %w (%s)", id, netName, err2, strings.TrimSpace(string(out2))))
		}
	}
	if out2, err2 := e.cmd(ctx, "network", "rm", netName).CombinedOutput(); err2 != nil {
		errs = append(errs, fmt.Errorf("sandbox egress network rm %s: %w (%s)", netName, err2, strings.TrimSpace(string(out2))))
	}
	return errors.Join(errs...)
}

// ReapOrphanNetworks implements NetworkReaper (startup reconcile): every
// network carrying the sandbox labels is an orphan at boot (the registry is
// empty and all labeled containers are being reaped anyway).
func (e *DockerEngine) ReapOrphanNetworks(ctx context.Context, labels map[string]string) (int, error) {
	args := []string{"network", "ls", "--format", "{{.Name}}"}
	for k, v := range labels {
		args = append(args, "--filter", "label="+k+"="+v)
	}
	out, err := e.cmd(ctx, args...).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("sandbox docker network ls: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	reaped := 0
	var errs []error
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := e.removeEgressNetwork(ctx, name); err != nil {
			errs = append(errs, err)
			continue
		}
		reaped++
	}
	return reaped, errors.Join(errs...)
}

// Create assembles `docker create` from the M32 isolation baseline
// (--network/--memory/--cpus/--read-only/tmpfs) plus the M82 hardening
// additions (cap-drop ALL, no-new-privileges, pids-limit), then starts the
// instance so it is ready to accept exec.
func (e *DockerEngine) Create(ctx context.Context, sandboxID string, p Profile, labels map[string]string) (Handle, error) {
	p = p.withDefaults()
	h := Handle{ID: sandboxID, SandboxID: sandboxID}

	network := dockerNetworkFor(p)
	if p.Network == NetworkEgress {
		// Per-sandbox egress lane: dedicated internal network + proxy as the
		// only peer (review 2026-08-26 #3 — no shared bridge, no ICC between
		// sandboxes).
		network = egressNetName(p, sandboxID)
		if err := e.createEgressNetwork(ctx, network, sandboxID, egressProxyHost(p.EgressProxy)); err != nil {
			return Handle{}, err
		}
		h.EgressNet = network
	}

	args := []string{
		"create",
		"--name", h.ID,
		"--network", network,
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
		if p.Network == NetworkEgress {
			_ = e.removeEgressNetwork(ctx, network)
		}
		return Handle{}, fmt.Errorf("sandbox docker create %s: %w (%s)", h.ID, err, strings.TrimSpace(string(out)))
	}
	if out, err := e.cmd(ctx, "start", h.ID).CombinedOutput(); err != nil {
		// Best-effort cleanup on a fresh context: the cleanCtx from the create
		// phase may have already expired; start failure is fatal, but leaking
		// the container or its dedicated egress network is worse.
		bg := context.Background()
		_ = e.cmd(bg, "rm", "-fv", h.ID).Run()
		if p.Network == NetworkEgress {
			_ = e.removeEgressNetwork(bg, network)
		}
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

// Destroy force-removes the instance and its anonymous volumes, then drops
// the per-sandbox egress network recorded on the Handle (review 2026-08-26
// #3; empty for none/full sandboxes). Failures are joined and returned
// (review 2026-08-26 r2 #4): teardown stays best-effort — the registry CAS
// makes destroy single-fire, so a returned error means real daemon trouble
// the manager must log, not routine "already gone".
func (e *DockerEngine) Destroy(ctx context.Context, h Handle) error {
	var errs []error
	if out, err := e.cmd(ctx, "rm", "-fv", h.ID).CombinedOutput(); err != nil {
		errs = append(errs, fmt.Errorf("sandbox docker rm %s: %w (%s)", h.ID, err, strings.TrimSpace(string(out))))
	}
	if h.EgressNet != "" {
		if err := e.removeEgressNetwork(ctx, h.EgressNet); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
