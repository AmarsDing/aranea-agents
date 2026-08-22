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
	"testing"
)

func TestWrapLinuxSandbox_NoBwrapLeavesCommand(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("bwrap"); err == nil {
		t.Skip("bwrap present; skip identity check")
	}
	cmd := exec.Command("echo", "hi")
	got := wrapLinuxSandbox(cmd, "/tmp")
	if got != cmd {
		t.Fatal("without bwrap the command must be unchanged")
	}
}
