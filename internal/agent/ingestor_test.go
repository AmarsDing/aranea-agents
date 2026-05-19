package agent

import (
	"context"
	"testing"

	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func TestNewBizSessionIngestorNilMemory(t *testing.T) {
	if NewBizSessionIngestor(nil) != nil {
		t.Fatal("expected nil ingestor without memory service")
	}
}

func TestBizSessionIngestorNoop(t *testing.T) {
	ing := &BizSessionIngestor{}
	sess := trpcsession.NewSession("app", "user", "sess-1")
	if err := ing.IngestSession(context.Background(), sess); err != nil {
		t.Fatalf("IngestSession() error = %v", err)
	}
}
