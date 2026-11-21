package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz/agentbridge"
	"aranea-agents/pkg/apierror"
)

// fleet:<工具 id> 的编码 agent 不走 ACP：运行台没登记时明确报不可用，而不是去 spawn 一个叫 "fleet:x" 的进程。
func TestACPSessionFactoryRoutesFleetAgents(t *testing.T) {
	prev := fleetBridge.Load()
	fleetBridge.Store(nil)
	t.Cleanup(func() { fleetBridge.Store(prev) })
	_, err := NewACPSessionFactory().Spawn(context.Background(), &agentbridge.CodingAgent{Command: "fleet:cursor"})
	if !apierror.IsCode(err, apierror.CodeUnavailable) {
		t.Fatalf("want Unavailable without fleet bridge, got %v", err)
	}
}
