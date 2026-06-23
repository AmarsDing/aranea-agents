package custom

import (
	"context"
	"errors"
	"fmt"
	"unicode/utf8"

	"aranea-agents/internal/biz"

	"aranea-agents/pkg/apierror"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const (
	// DefaultReadChunkSize is the default number of characters returned by
	// read_tool_result per call. It must stay well below
	// biz.ToolResultSizeThreshold (50 000) so that the ToolResultGate
	// BeforeModel hook does not re-persist and re-truncate the chunk,
	// which would create an infinite loop.
	DefaultReadChunkSize = 10000

	// MaxReadChunkSize caps the per-request limit to prevent accidental
	// context overflow even when the caller specifies a large limit.
	MaxReadChunkSize = 40000
)

type readToolResultInput struct {
	BlobID string `json:"blob_id" jsonschema:"description=The blob ID of the persisted tool result to retrieve,required"`
	Offset int    `json:"offset,omitempty" jsonschema:"description=Character offset to start reading from (0-based). Defaults to 0."`
	Limit  int    `json:"limit,omitempty" jsonschema:"description=Maximum number of characters to return. Defaults to 10000. Capped at 40000."`
}

type readToolResultOutput struct {
	Content   string `json:"content"`
	Found     bool   `json:"found"`
	Offset    int    `json:"offset"`
	TotalSize int    `json:"total_size"`
	HasMore   bool   `json:"has_more"`
}

func NewReadToolResultTool(reader biz.ToolResultBlobReader) *trpcfunction.FunctionTool[readToolResultInput, readToolResultOutput] {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input readToolResultInput) (readToolResultOutput, error) {
			if input.BlobID == "" {
				return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "blob_id is required")
			}
			if input.Offset < 0 {
				return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "offset must be >= 0")
			}

			blob, err := reader.GetBlob(ctx, input.BlobID)
			if err != nil {
				// Distinguish "not found" (return Found:false) from real DB
				// failures (propagate as Internal error). Swallowing DB errors
				// would let the LLM mistake a DB outage for "result not yet
				// persisted" and retry indefinitely.
				var ae *apierror.Error
				if errors.As(err, &ae) && ae.Code == apierror.CodeNotFound {
					return readToolResultOutput{Found: false}, nil
				}
				return readToolResultOutput{}, apierror.Internal(apierror.DomainTool, "read blob: "+err.Error())
			}
			if blob == nil {
				return readToolResultOutput{Found: false}, nil
			}

			// Enforce session ownership: a blob can only be read by the
			// session that created it. This prevents cross-session
			// information leakage when blob IDs are guessed or leaked.
			// Deny by default: if the current session ID is unavailable
			// (e.g., tool invoked outside agent runtime) or the blob has
			// no owning session, access is refused.
			currentSessionID := sessionIDFromContext(ctx)
			if currentSessionID == "" || blob.SessionID == "" || currentSessionID != blob.SessionID {
				return readToolResultOutput{Found: false}, nil
			}

			limit := DefaultReadChunkSize
			if input.Limit > 0 {
				limit = input.Limit
			}
			if limit > MaxReadChunkSize {
				limit = MaxReadChunkSize
			}

			content, offset, totalSize, hasMore := sliceContent(blob.FullContent, input.Offset, limit)

			return readToolResultOutput{
				Content:   content,
				Found:     true,
				Offset:    offset,
				TotalSize: totalSize,
				HasMore:   hasMore,
			}, nil
		},
		trpcfunction.WithName("read_tool_result"),
		trpcfunction.WithDescription(
			fmt.Sprintf(
				"Retrieve a chunk of a previously persisted tool result by its blob_id. "+
					"Returns up to %d characters per call (configurable via limit, max %d). "+
					"Use offset and has_more to paginate through large results. "+
					"This avoids overflowing the conversation context with a single massive output.",
				DefaultReadChunkSize, MaxReadChunkSize,
			),
		),
	)
}

// sessionIDFromContext extracts the current session ID from the agent
// invocation context. Returns empty string if unavailable.
func sessionIDFromContext(ctx context.Context) string {
	inv, ok := trpcagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.Session == nil {
		return ""
	}
	return inv.Session.ID
}

// sliceContent extracts a rune-safe slice of fullContent starting at the given
// character offset, returning at most limit characters. It returns the sliced
// content, the actual offset used, the total rune count, and whether more
// content remains beyond the returned chunk.
//
// Instead of converting the entire string to []rune (which would allocate
// ~4× the string length in memory for large content), this implementation
// walks the UTF-8 byte sequence to find the start and end boundaries,
// then slices the underlying string directly. This keeps peak memory
// proportional to the requested chunk, not the total content size.
func sliceContent(fullContent string, offset, limit int) (content string, actualOffset, totalSize int, hasMore bool) {
	// Count total runes without allocating.
	totalRunes := utf8.RuneCountInString(fullContent)
	totalSize = totalRunes

	if offset >= totalRunes {
		return "", offset, totalSize, false
	}

	// Walk to the byte offset of the start rune.
	byteStart := 0
	for i := 0; i < offset; i++ {
		_, size := utf8.DecodeRuneInString(fullContent[byteStart:])
		byteStart += size
	}

	// Walk to the byte offset of the end rune (start + limit, capped).
	endRune := offset + limit
	if endRune > totalRunes {
		endRune = totalRunes
	}
	byteEnd := byteStart
	for i := offset; i < endRune; i++ {
		_, size := utf8.DecodeRuneInString(fullContent[byteEnd:])
		byteEnd += size
	}

	hasMore = endRune < totalRunes
	content = fullContent[byteStart:byteEnd]
	return content, offset, totalSize, hasMore
}
