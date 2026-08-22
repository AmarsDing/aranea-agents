//go:build windows

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
	"context"
	"testing"
	"time"
)

func TestProcessSandbox_JobKillsChild(t *testing.T) {
	t.Parallel()
	mgr := newManager()
	mgr.sandbox = true
	res, err := mgr.exec(context.Background(), execParams{
		Command:    `ping 127.0.0.1 -n 30`,
		Background: true,
		TimeoutS:   intPtr(60),
	})
	if err != nil {
		t.Fatalf("start sandboxed ping: %v", err)
	}
	if res.SessionID == "" || res.PID == 0 {
		t.Fatalf("missing session: %+v", res)
	}
	mgr.mu.Lock()
	sess := mgr.sessions[res.SessionID]
	mgr.mu.Unlock()
	if sess == nil || sess.sandbox == nil {
		t.Fatal("session must hold a job sandbox")
	}
	if err := sess.kill(context.Background(), 200*time.Millisecond); err != nil {
		t.Fatalf("kill job: %v", err)
	}
	select {
	case <-sess.doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("sandboxed child did not exit after job kill")
	}
}
