package memory

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

func TestPrepareConsolidationMemories_SkipsGreetings(t *testing.T) {
	kept, ok := prepareConsolidationMemories([]*trpcmemory.Entry{
		makeEntry("a", "hi"),
		makeEntry("b", "谢谢"),
		makeEntry("c", "ok"),
	})
	if ok || len(kept) != 0 {
		t.Fatalf("greetings must not be high-signal, kept=%d ok=%v", len(kept), ok)
	}
}

func TestPrepareConsolidationMemories_KeepsPreferences(t *testing.T) {
	kept, ok := prepareConsolidationMemories([]*trpcmemory.Entry{
		makeEntry("a", "hello"),
		makeEntry("b", "User likes Go"),
	})
	if !ok || len(kept) != 1 || kept[0].ID != "b" {
		t.Fatalf("expected only preference memory, kept=%d ok=%v", len(kept), ok)
	}
}

func TestPrepareConsolidationMemories_RedactsSecrets(t *testing.T) {
	kept, ok := prepareConsolidationMemories([]*trpcmemory.Entry{
		makeEntry("a", `User prefers dark mode. api_key: sk-abcdefgh`),
	})
	if !ok || len(kept) != 1 {
		t.Fatalf("mixed secret+pref should stay, ok=%v n=%d", ok, len(kept))
	}
	if strings.Contains(kept[0].Memory.Memory, "sk-abcdefgh") {
		t.Fatalf("secret leaked to consolidator: %q", kept[0].Memory.Memory)
	}
	if !strings.Contains(kept[0].Memory.Memory, "dark mode") {
		t.Fatalf("durable text lost after redact: %q", kept[0].Memory.Memory)
	}
}

func TestPrepareConsolidationMemories_SecretOnlyIsNoop(t *testing.T) {
	kept, ok := prepareConsolidationMemories([]*trpcmemory.Entry{
		makeEntry("a", `api_key: sk-abcdefgh`),
	})
	if ok || len(kept) != 0 {
		t.Fatalf("secret-only must skip LLM, kept=%d ok=%v", len(kept), ok)
	}
}

func TestSleepTime_Consolidate_NoHighSignalSkipsLLM(t *testing.T) {
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {makeEntry("mem-1", "hi"), makeEntry("mem-2", "ok")},
		},
	}
	llm := &fakeModel{response: buildLLMResponse(`{"operations":[{"type":"reflect","reflection":"should not run"}]}`)}
	svc := NewSleepTimeService(ms, llm, nil, loggateway.NewNoop())
	if err := svc.Consolidate(context.Background(), trpcmemory.UserKey{AppName: "a", UserID: "user-1"}); err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
	if len(ms.added) != 0 {
		t.Fatalf("low-signal must not write, added=%v", ms.added)
	}
}

func TestExecuteOperations_RefusesSecretWrite(t *testing.T) {
	ms := &fakeMemoryService{entries: map[string][]*trpcmemory.Entry{}}
	svc := NewSleepTimeService(ms, nil, nil, loggateway.NewNoop())
	err := svc.executeOperations(context.Background(), trpcmemory.UserKey{AppName: "a", UserID: "u"},
		[]ConsolidationOperation{{
			Type:       "reflect",
			Reflection: `remember api_key: sk-abcdefgh`,
		}})
	if err != nil {
		t.Fatalf("executeOperations: %v", err)
	}
	if len(ms.added) != 0 {
		t.Fatalf("secret reflection must be refused, added=%v", ms.added)
	}
}
