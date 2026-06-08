package agent

import (
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestMemoryCueResult_IsEmpty(t *testing.T) {
	if (&MemoryCueResult{}).IsEmpty() != true {
		t.Error("empty result should be empty")
	}
	if (&MemoryCueResult{L1Cue: "test"}).IsEmpty() != false {
		t.Error("result with L1Cue should not be empty")
	}
	if (&MemoryCueResult{RecallCue: "test"}).IsEmpty() != false {
		t.Error("result with RecallCue should not be empty")
	}
}

func TestMemoryCueResult_JoinCues(t *testing.T) {
	r := &MemoryCueResult{L1Cue: "L1", RecallCue: "Recall"}
	if r.JoinCues() != "L1\n\nRecall" {
		t.Errorf("unexpected JoinCues result: %q", r.JoinCues())
	}
	if (&MemoryCueResult{L1Cue: "L1"}).JoinCues() != "L1" {
		t.Errorf("unexpected JoinCues result with only L1: %q", (&MemoryCueResult{L1Cue: "L1"}).JoinCues())
	}
}

func TestIsMemoryInjectMessage(t *testing.T) {
	msg := trpcmodel.NewSystemMessage(memoryInjectCueContent("test cue"))
	if !isMemoryInjectMessage(msg) {
		t.Error("message with marker should be identified")
	}
	plainMsg := trpcmodel.NewSystemMessage("plain content")
	if isMemoryInjectMessage(plainMsg) {
		t.Error("plain message should not be identified as memory inject")
	}
}
