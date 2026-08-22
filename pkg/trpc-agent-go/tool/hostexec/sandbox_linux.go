//go:build linux

//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package hostexec

import (
	"os/exec"
)

func prepareOSSandbox(_ *exec.Cmd) {}

func attachProcessSandbox(_ *exec.Cmd) (processSandbox, error) {
	return noopSandbox{}, nil
}

// wrapLinuxSandbox prefixes the command with bubblewrap --die-with-parent
// when bwrap is on PATH so the tree dies with the hostexec session. FS
// bind policy is left to the workspace path hook; this is process lifetime
// containment, not a full Seatbelt profile.
func wrapLinuxSandbox(cmd *exec.Cmd, workdir string) *exec.Cmd {
	if cmd == nil {
		return cmd
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return cmd
	}
	if workdir == "" {
		workdir = cmd.Dir
	}
	args := make([]string, 0, 6+len(cmd.Args))
	args = append(args, "--die-with-parent", "--new-session")
	if workdir != "" {
		args = append(args, "--chdir", workdir)
	}
	args = append(args, cmd.Args...)
	wrapped := exec.Command(bwrap, args...)
	wrapped.Dir = cmd.Dir
	wrapped.Env = cmd.Env
	wrapped.Stdin = cmd.Stdin
	wrapped.Stdout = cmd.Stdout
	wrapped.Stderr = cmd.Stderr
	wrapped.ExtraFiles = cmd.ExtraFiles
	return wrapped
}

type noopSandbox struct{}

func (noopSandbox) Kill() error  { return nil }
func (noopSandbox) Close() error { return nil }
