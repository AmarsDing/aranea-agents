package testexec

import (
	"context"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// newMockMemoryTools returns a set of mock memory tools for the test harness.
// In the test harness there is no MemoryService from the runner context,
// so these return stub responses.
func newMockMemoryTools() []trpctool.Tool {
	return []trpctool.Tool{
		mockMemorySearchTool(),
		mockMemoryLoadTool(),
		mockMemoryAddTool(),
		mockMemoryUpdateTool(),
		mockMemoryDeleteTool(),
	}
}

type mockMemorySearchInput struct {
	Query string `json:"query" description:"The search query"`
}

type mockMemorySearchOutput struct {
	Results []map[string]any `json:"results"`
}

func mockMemorySearchTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockMemorySearchInput) (mockMemorySearchOutput, error) {
			return mockMemorySearchOutput{Results: []map[string]any{}}, nil
		},
		trpcfunction.WithName("memory_search"),
		trpcfunction.WithDescription("Search memory for relevant information (test mock)."),
	)
}

type mockMemoryLoadInput struct {
	Key string `json:"key" description:"The memory key to load"`
}

type mockMemoryLoadOutput struct {
	Value string `json:"value"`
	Found bool   `json:"found"`
}

func mockMemoryLoadTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockMemoryLoadInput) (mockMemoryLoadOutput, error) {
			return mockMemoryLoadOutput{Found: false}, nil
		},
		trpcfunction.WithName("memory_load"),
		trpcfunction.WithDescription("Load a value from memory by key (test mock)."),
	)
}

type mockMemoryAddInput struct {
	Key   string `json:"key" description:"The memory key"`
	Value string `json:"value" description:"The value to store"`
}

type mockMemoryAddOutput struct {
	Success bool `json:"success"`
}

func mockMemoryAddTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockMemoryAddInput) (mockMemoryAddOutput, error) {
			return mockMemoryAddOutput{Success: true}, nil
		},
		trpcfunction.WithName("memory_add"),
		trpcfunction.WithDescription("Add a value to memory (test mock)."),
	)
}

type mockMemoryUpdateInput struct {
	Key   string `json:"key" description:"The memory key"`
	Value string `json:"value" description:"The new value"`
}

type mockMemoryUpdateOutput struct {
	Success bool `json:"success"`
}

func mockMemoryUpdateTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockMemoryUpdateInput) (mockMemoryUpdateOutput, error) {
			return mockMemoryUpdateOutput{Success: true}, nil
		},
		trpcfunction.WithName("memory_update"),
		trpcfunction.WithDescription("Update a value in memory (test mock)."),
	)
}

type mockMemoryDeleteInput struct {
	Key string `json:"key" description:"The memory key to delete"`
}

type mockMemoryDeleteOutput struct {
	Success bool `json:"success"`
}

func mockMemoryDeleteTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockMemoryDeleteInput) (mockMemoryDeleteOutput, error) {
			return mockMemoryDeleteOutput{Success: true}, nil
		},
		trpcfunction.WithName("memory_delete"),
		trpcfunction.WithDescription("Delete a value from memory (test mock)."),
	)
}
