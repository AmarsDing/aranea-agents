package agent

import (
	"aranea-agents/internal/biz"
	"context"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestAgentFactory_EnsureAgent_SameDomainMissionReuse(t *testing.T) {
	store := newFakeFactoryAgentStore()

	store.agents["factory-existing"] = biz.Agent{
		AgentKey: "factory-existing", DisplayName: "文学写手", Status: "active",
		DomainPath: "创作/文学", MissionStatement: "中文诗歌散文创作",
	}
	bus := &factoryCaptureBus{}
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), bus)
	factory.embedder = embedFunc(func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1, 0, 0}
		}
		return out, nil
	})

	key, err := factory.EnsureAgent(context.Background(), biz.TaskProfile{
		DomainPath: "创作/文学", TaskDescription: "写一首春天的诗",
	})
	if err != nil {
		t.Fatalf("EnsureAgent err: %v", err)
	}
	if key != "factory-existing" {
		t.Errorf("key=%q want factory-existing（同域使命相似复用）", key)
	}
	if len(store.agents) != 1 {
		t.Errorf("agents=%d want 1（未创建新 Agent）", len(store.agents))
	}
	if len(bus.getPublished()) != 0 {
		t.Errorf("published=%d want 0（复用不发事件）", len(bus.getPublished()))
	}
}

func TestAgentFactory_EnsureAgent_DomainReuseBelowThreshold(t *testing.T) {
	store := newFakeFactoryAgentStore()
	store.agents["factory-existing"] = biz.Agent{
		AgentKey: "factory-existing", DisplayName: "科幻写手", Status: "active",
		DomainPath: "创作/文学", MissionStatement: "硬科幻小说创作",
	}
	model := &fakeFactoryModel{
		responses: []*trpcmodel.Response{
			{Choices: []trpcmodel.Choice{{Message: trpcmodel.Message{Content: validFactoryJSONResponse()}}}},
		},
	}
	factory := newTestAgentFactory(model, store, defaultFactoryTemplates(), &factoryCaptureBus{})
	factory.embedder = embedFunc(func(_ context.Context, texts []string) ([][]float32, error) {
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1, 0, 0}
		}
		if len(out) > 1 {
			out[1] = []float32{0, 1, 0}
		}
		return out, nil
	})

	key, err := factory.EnsureAgent(context.Background(), biz.TaskProfile{
		DomainPath: "创作/文学", TaskDescription: "写一首春天的诗",
	})
	if err != nil {
		t.Fatalf("EnsureAgent err: %v", err)
	}
	if key == "factory-existing" {
		t.Error("similarity below 0.85 must create new agent")
	}
	if len(store.agents) != 2 {
		t.Errorf("agents=%d want 2（新建落库）", len(store.agents))
	}

	created, _ := store.GetAgentByAgentKey(context.Background(), key)
	if created.DomainPath != "创作" {
		t.Errorf("created.DomainPath=%q want %q（definition 归一化落库）", created.DomainPath, "创作")
	}
	if created.MissionStatement == "" {
		t.Error("created.MissionStatement must not be empty（出生登记）")
	}
}
