package sandbox

import (
	"archive/tar"
	"bytes"
	"context"
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
	case err != nil || res.ExitCode != 0:
		status = "error"
	case res.TimedOut:
		status = "timeout"
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

// WriteFile writes content to path inside the sandbox (parent dirs must exist
// in a writable mount — /tmp or /out under the P0 profile).
func (l *Lease) WriteFile(ctx context.Context, filePath string, content []byte) error {
	e, err := l.leasedEntry()
	if err != nil {
		return err
	}
	dir, name := path.Split(filePath)
	if name == "" {
		return ErrNotFound
	}
	if dir == "" {
		dir = "/"
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(content)),
		ModTime: l.m.now(),
	}); err != nil {
		return err
	}
	if _, err := tw.Write(content); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return l.m.engine.CopyTo(ctx, e.handle, dir, &buf)
}

// ReadFile reads a single file from the sandbox.
func (l *Lease) ReadFile(ctx context.Context, filePath string) ([]byte, error) {
	e, err := l.leasedEntry()
	if err != nil {
		return nil, err
	}
	rc, err := l.m.engine.CopyFrom(ctx, e.handle, filePath)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		return io.ReadAll(tr)
	}
}

// CopyDirFrom streams a sandbox directory out as a tar archive (artifact
// collection path used by the codeexecutor pooled backend). The caller owns
// untarring via UntarToDir.
func (l *Lease) CopyDirFrom(ctx context.Context, dir string) (io.ReadCloser, error) {
	e, err := l.leasedEntry()
	if err != nil {
		return nil, err
	}
	// Trailing "/." copies the directory CONTENTS (docker cp semantics).
	src := strings.TrimSuffix(dir, "/") + "/."
	return l.m.engine.CopyFrom(ctx, e.handle, src)
}

// Renew extends the lease deadline, capped at TTL.Max from now (design §5.1).
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
// skipped (sandbox artifacts are regular files/dirs; links are an escape risk).
func UntarFiles(r io.Reader, writeFile func(relPath string, content []byte) error) error {
	tr := tar.NewReader(r)
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
		content, err := io.ReadAll(tr)
		if err != nil {
			return err
		}
		if err := writeFile(hdr.Name, content); err != nil {
			return err
		}
	}
}
