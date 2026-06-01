package agent

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestBizSessionIngestor_IngestSession(t *testing.T) {
	ing := &BizSessionIngestor{lg: loggateway.Global()}
	err := ing.IngestSession(context.Background(), &trpcsession.Session{
		ID:      "sess-1",
		AppName: "aranea",
		UserID:  "u1",
	}, trpcsession.WithIngestRunID("sess-1"), trpcsession.WithIngestAgentID("agent-a"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewBizSessionIngestor_NilMemory(t *testing.T) {
	if NewBizSessionIngestor(nil, nil) != nil {
		t.Fatal("expected nil ingestor without memory")
	}
}
