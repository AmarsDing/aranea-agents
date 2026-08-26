package sandbox

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"sync/atomic"
	"time"
)

// Lease is the exclusive consumer handle over one sandbox (design §5.1).
// It is not reusable: Release destroys the sandbox and further calls fail.
type Lease struct {
	m        *Manager
	id       string
	profile  string
	released atomic.Bool
}

// SandboxID returns the platform sandbox id.
func (l *Lease) SandboxID() string { return l.id }

// Profile returns the profile name this lease was acquired under.
func (l *Lease) Profile() string { return l.profile }

// Alive reports whether the lease is still registered as leased (false after
// manager-side idle/TTL/force destroy). Consumers use it to disambiguate
// ErrNotFound from file-level misses inside a live sandbox.
func (l *Lease) Alive() bool {
	_, err := l.leasedEntry()
	return err == nil
}

// leasedEntry returns the registry entry if the lease is still live.
func (l *Lease) leasedEntry() (*entry, error) {
	e, ok := l.m.registry.get(l.id)
	if !ok || e.entryState != StateLeased {
		return nil, ErrNotFound
	}
	return e, nil
}

// Exec runs one command inside the sandbox (design §3.2).
func (l *Lease) Exec(ctx context.Context, spec ExecSpec) (ExecResult, error) {
	e, err := l.leasedEntry()
	if err != nil {
		return ExecResult{}, err
	}
	start := l.m.now()
	res, err := l.m.engine.Exec(ctx, e.handle, spec)
	status := "ok"
	switch {
	// TimedOut first: both real timeout paths (ctx-killed CLI → err!=nil,
	// ExitCode=-1; in-container timeout wrapper → ExitCode=124) carry a
	// non-zero exit, so checking err/ExitCode first makes "timeout"
	// unreachable (review 2026-08-26 #5).
	case res.TimedOut:
		status = "timeout"
	case err != nil || res.ExitCode != 0:
		status = "error"
	}
	execDuration.WithLabelValues(l.profile, status).Observe(l.m.now().Sub(start).Seconds())
	switch status {
	case "ok":
		l.m.st.execOK.Add(1)
	case "timeout":
		l.m.st.execTimeout.Add(1)
	default:
		l.m.st.execError.Add(1)
	}
	l.m.registry.touch(l.id, l.m.now())
	return res, err
}

// WriteFile writes content to path inside the sandbox via `cat > path` over
// exec stdin (review 2026-08-26 r2 #2): the file is created by the
// container's own user (base image USER sandbox), so later code execution can
// modify or delete it — the previous tar/docker-cp path wrote root-owned
// files the sandbox user could not touch, and buffered the whole payload
// twice in host memory. Parent dirs must exist in a writable mount (/tmp or
// /workspace/out under the P0 profile).
func (l *Lease) WriteFile(ctx context.Context, filePath string, content []byte) error {
	e, err := l.leasedEntry()
	if err != nil {
		return err
	}
	// Any live-lease operation is activity: refresh the idle clock (review
	// 2026-08-26 #2 — touch was Exec-only, so fs-only sessions were destroyed
	// by the idle GC while actively in use).
	l.m.registry.touch(l.id, l.m.now())
	_, name := path.Split(filePath)
	if name == "" {
		return ErrNotFound
	}
	// Positional-param quoting keeps arbitrary (model-supplied) paths
	// injection-safe; stdin is streamed raw, so binary content is preserved.
	res, err := l.m.engine.Exec(ctx, e.handle, ExecSpec{
		Argv:  []string{"sh", "-c", "cat > \"$1\"", "sh", filePath},
		Stdin: string(content),
	})
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("sandbox write %s: exit %d: %s", filePath, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

// ReadFile reads a single regular file from the sandbox, streaming at most
// maxBytes (truncated=true when the file is larger; maxBytes<=0 falls back to
// ReadFileMaxBytesDefault). A directory path returns ErrNotRegular (review
// 2026-08-26 r2 #6 — previously it surfaced as ErrNotFound or, worse, the
// first regular child of the directory).
func (l *Lease) ReadFile(ctx context.Context, filePath string, maxBytes int64) ([]byte, bool, error) {
	e, err := l.leasedEntry()
	if err != nil {
		return nil, false, err
	}
	l.m.registry.touch(l.id, l.m.now()) // activity refresh, see WriteFile
	if maxBytes <= 0 {
		maxBytes = ReadFileMaxBytesDefault
	}
	rc, err := l.m.engine.CopyFrom(ctx, e.handle, filePath)
	if err != nil {
		return nil, false, err
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	hdr, err := tr.Next()
	if err == io.EOF {
		return nil, false, ErrNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if hdr.Typeflag != tar.TypeReg {
		return nil, false, ErrNotRegular
	}
	// +1 byte detects truncation without reading the whole file into memory.
	data, err := io.ReadAll(io.LimitReader(tr, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], true, nil
	}
	return data, false, nil
}

// CopyDirFrom streams a sandbox directory out as a tar archive (artifact
// collection path used by the codeexecutor pooled backend). The caller owns
// untarring via UntarToDir.
func (l *Lease) CopyDirFrom(ctx context.Context, dir string) (io.ReadCloser, error) {
	e, err := l.leasedEntry()
	if err != nil {
		return nil, err
	}
	l.m.registry.touch(l.id, l.m.now()) // activity refresh, see WriteFile
	// Trailing "/." copies the directory CONTENTS (docker cp semantics).
	src := strings.TrimSuffix(dir, "/") + "/."
	return l.m.engine.CopyFrom(ctx, e.handle, src)
}

// Renew extends the lease deadline by extend — added to the CURRENT deadline
// (accumulate semantics), capped at TTL.Max from now (design §5.1). Called
// after every successful use, so the cap is what bounds long-lived sessions.
func (l *Lease) Renew(ctx context.Context, extend time.Duration) error {
	if extend <= 0 {
		return nil
	}
	maxDeadline := l.m.now().Add(l.m.cfg.TTL.Max)
	if _, ok := l.m.registry.renewDeadline(l.id, extend, maxDeadline); !ok {
		return ErrNotFound
	}
	return nil
}

// Release returns the lease: the sandbox is destroyed immediately and is
// NEVER recycled into the pool (ADR-82-2). Idempotent.
func (l *Lease) Release(ctx context.Context) error {
	if l.released.CompareAndSwap(false, true) {
		l.m.destroy(l.id, ReasonRelease)
	}
	return nil
}

// UntarFiles walks a tar stream (from CopyDirFrom) and hands each regular
// file to writeFile with its archive-relative path. Symlinks/hardlinks are
// skipped (sandbox artifacts are regular files/dirs; links are an escape
// risk). The cumulative payload is capped at maxTotalBytes (review 2026-08-26
// r2 #5 — a runaway sandbox writing unbounded output must not OOM the host
// during artifact collection); exceeding it aborts with ErrTooLarge.
func UntarFiles(r io.Reader, maxTotalBytes int64, writeFile func(relPath string, content []byte) error) error {
	tr := tar.NewReader(r)
	var total int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		remaining := maxTotalBytes - total
		if remaining < 0 {
			return ErrTooLarge
		}
		// +1 byte detects overflow of this file against the remaining budget.
		content, err := io.ReadAll(io.LimitReader(tr, remaining+1))
		if err != nil {
			return err
		}
		if int64(len(content)) > remaining {
			return ErrTooLarge
		}
		total += int64(len(content))
		if err := writeFile(hdr.Name, content); err != nil {
			return err
		}
	}
}
