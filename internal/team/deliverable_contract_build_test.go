package team

import (
	"testing"
)

func TestParseDefinition_DeliverableContract(t *testing.T) {
	raw := `{
		"version":1,"mode":"sequential",
		"enable_state_deliverable":true,
		"deliverable_contract":{"entries":[{"topic":"design","required":true,"required_keys":["arch"]}]},
		"members":[{"agent_id":"a1"}]
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if def.DeliverableContract == nil {
		t.Fatal("expected DeliverableContract parsed")
	}
	entry := def.DeliverableContract.EntryForTopic("design")
	if entry == nil || !entry.Required || len(entry.RequiredKeys) != 1 {
		t.Fatalf("unexpected contract: %+v", def.DeliverableContract)
	}
}

func TestParseDefinition_NoDeliverableContract(t *testing.T) {
	def, err := ParseDefinition(`{"version":1,"mode":"sequential","members":[{"agent_id":"a1"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if def.DeliverableContract != nil {
		t.Fatalf("expected nil contract, got %+v", def.DeliverableContract)
	}
}

func TestParseDefinition_InvalidDeliverableContract(t *testing.T) {
	// 契约 JSON 非法（entries 类型错误）应 fail-fast 而非静默丢弃
	if _, err := ParseDefinition(`{"mode":"sequential","deliverable_contract":{"entries":"oops"},"members":[{"agent_id":"a1"}]}`); err == nil {
		t.Fatal("invalid deliverable_contract should fail definition parse")
	}
}

func TestDeliverableToolsForDef_Disabled(t *testing.T) {
	def := Definition{Mode: "sequential"}
	if tools := deliverableToolsForDef(def); tools != nil {
		t.Fatalf("EnableStateDeliverable=false → no tools, got %d", len(tools))
	}
}

func TestDeliverableToolsForDef_EnabledNoContract(t *testing.T) {
	def := Definition{Mode: "sequential", EnableStateDeliverable: true}
	tools := deliverableToolsForDef(def)
	if len(tools) != 3 {
		t.Fatalf("expected set/get/ack 3 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Declaration().Name] = true
	}
	for _, want := range []string{"set_deliverable", "get_deliverable", "ack_deliverable"} {
		if !names[want] {
			t.Fatalf("missing tool %q in %v", want, names)
		}
	}
}

func TestParallelDeliverableAdvisory(t *testing.T) {
	// parallel + deliverable → advisory warning text
	def := Definition{Mode: "parallel", EnableStateDeliverable: true}
	if msg := parallelDeliverableAdvisory(def); msg == "" {
		t.Fatal("parallel + EnableStateDeliverable should produce advisory")
	}
	// sequential + deliverable → no advisory
	def2 := Definition{Mode: "sequential", EnableStateDeliverable: true}
	if msg := parallelDeliverableAdvisory(def2); msg != "" {
		t.Fatalf("sequential should not produce advisory, got %q", msg)
	}
	// parallel without deliverable → no advisory
	def3 := Definition{Mode: "parallel"}
	if msg := parallelDeliverableAdvisory(def3); msg != "" {
		t.Fatalf("parallel without deliverable should not produce advisory, got %q", msg)
	}
}
