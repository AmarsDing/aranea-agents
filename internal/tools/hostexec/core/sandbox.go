//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package hostexec

// processSandbox is OS-level containment for one spawned command (Job
// Object on Windows; optional bubblewrap wrap on Linux). Kill tears the
// tree down; Close releases handles (Windows Job Objects use
// KILL_ON_JOB_CLOSE so leftover children die with the handle).
type processSandbox interface {
	Kill() error
	Close() error
}
