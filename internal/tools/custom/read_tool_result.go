package custom

import (
	"context"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type readToolResultInput struct {
	BlobID string `json:"blob_id" description:"The blob ID of the persisted tool result to retrieve"`
}

type readToolResultOutput struct {
	Content string `json:"content"`
	Found   bool   `json:"found"`
}

func NewReadToolResultTool(reader biz.ToolResultBlobReader) *trpcfunction.FunctionTool[readToolResultInput, readToolResultOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input readToolResultInput) (readToolResultOutput, error) {
			if input.BlobID == "" {
				return readToolResultOutput{}, kerrors.BadRequest("TOOL_RESULT", "blob_id is required")
			}
			blob, err := reader.GetBlob(ctx, input.BlobID)
			if err != nil {
				return readToolResultOutput{Found: false}, nil
			}
			return readToolResultOutput{
				Content: blob.FullContent,
				Found:   true,
			}, nil
		},
		trpcfunction.WithName("read_tool_result"),
		trpcfunction.WithDescription("Retrieve the full content of a previously persisted tool result by its blob_id. Use this when you need to see the complete output that was truncated in the conversation."),
	)
}
