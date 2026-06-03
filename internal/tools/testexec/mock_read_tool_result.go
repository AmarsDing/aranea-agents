package testexec

import (
	"context"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type mockReadToolResultInput struct {
	BlobID string `json:"blob_id" description:"The blob ID of the persisted tool result to retrieve"`
}

type mockReadToolResultOutput struct {
	Content string `json:"content"`
	Found   bool   `json:"found"`
}

// newMockReadToolResultTool creates a read_tool_result tool backed by an in-memory mock.
// In the test harness there are no persisted blobs, so it always returns found=false.
func newMockReadToolResultTool() trpctool.CallableTool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input mockReadToolResultInput) (mockReadToolResultOutput, error) {
			if input.BlobID == "" {
				return mockReadToolResultOutput{}, kerrors.BadRequest("TOOL_RESULT", "blob_id is required")
			}
			return mockReadToolResultOutput{Found: false}, nil
		},
		trpcfunction.WithName("read_tool_result"),
		trpcfunction.WithDescription("Retrieve the full content of a previously persisted tool result by its blob_id (test mock)."),
	)
}
