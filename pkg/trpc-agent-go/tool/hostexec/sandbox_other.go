//go:build !windows && !linux

//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package hostexec

import "os/exec"

func wrapLinuxSandbox(cmd *exec.Cmd, _ string) *exec.Cmd { return cmd }

func prepareOSSandbox(_ *exec.Cmd) {}

func attachProcessSandbox(_ *exec.Cmd) (processSandbox, error) {
	return noopSandbox{}, nil
}

type noopSandbox struct{}

func (noopSandbox) Kill() error  { return nil }
func (noopSandbox) Close() error { return nil }
