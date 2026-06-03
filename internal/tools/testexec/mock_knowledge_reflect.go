package testexec

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type mockKnowledgeReflectInput struct {
	Query         string   `json:"query" description:"The query to reflect on"`
	CollectionIDs []string `json:"collection_ids" description:"The collection IDs to search"`
}

type mockKnowledgeReflectOutput struct {
	Reflection string `json:"reflection"`
	Found      bool   `json:"found"`
}

// newMockKnowledgeReflectTool creates a knowledge_reflect tool backed by a mock.
// In the test harness there is no retriever, so it returns a stub reflection.
func newMockKnowledgeReflectTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockKnowledgeReflectInput) (mockKnowledgeReflectOutput, error) {
			if input.Query == "" {
				return mockKnowledgeReflectOutput{}, kerrors.BadRequest("KNOWLEDGE_REFLECT", "query is required")
			}
			return mockKnowledgeReflectOutput{
				Reflection: "mock reflection for: " + input.Query,
				Found:      true,
			}, nil
		},
		trpcfunction.WithName("knowledge_reflect"),
		trpcfunction.WithDescription("Reflect on knowledge to find relevant information (test mock)."),
	)
}
