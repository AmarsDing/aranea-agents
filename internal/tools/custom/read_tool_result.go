package custom

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/toolspill"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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

// ReadToolResultOptions bundles the optional P0-2 dependencies of
// read_tool_result (batch-B wire points; batch A leaves them unwired).
type ReadToolResultOptions struct {
	// Spill resolves spill://<store>/<id> locators written by the tool-result
	// spill path (design §3.4). Nil keeps the gray gate (see below).
	Spill toolspill.Store
	// Logger receives structured logs; nil = noop.
	Logger loggateway.Logger
}

// NewReadToolResultTool builds the read_tool_result function tool. The
// variadic opts keep the historical single-argument call compiling: with no
// options the tool resolves blob IDs only.
//
// P0-2 locator branch (design §3.8④ + §12.2 P0-2④, batch-A gray gate):
// blob_id values carrying the spill:// scheme are parsed and validated
// first; while no spill store is wired (opts absent or Spill nil) they are
// answered with BadRequest — never a silent empty result the model could
// mistake for an empty payload. Once wired, retrieval enforces the store's
// own workspace containment: only spill://fs locators under the store root,
// session-hash segment matching the calling session (cross-session reads
// answer Found:false), symlink components rejected, so any path escaping the
// private spill directory is refused.
func NewReadToolResultTool(reader biz.ToolResultBlobReader, opts ...*ReadToolResultOptions) *trpcfunction.FunctionTool[readToolResultInput, readToolResultOutput] {
	var cfg ReadToolResultOptions
	if len(opts) > 0 && opts[0] != nil {
		cfg = *opts[0]
	}
	lg := cfg.Logger
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, input readToolResultInput) (readToolResultOutput, error) {
			if input.BlobID == "" {
				return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "blob_id is required")
			}
			if input.Offset < 0 {
				return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "offset must be >= 0")
			}

			if strings.HasPrefix(input.BlobID, toolspill.Scheme) {
				return readSpillResult(ctx, cfg.Spill, lg, input)
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

			limit := clampReadLimit(input.Limit)

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

// readSpillResult serves the spill:// locator branch of read_tool_result.
// Parse-and-validate happens unconditionally; the store then enforces
// ownership and containment (see NewReadToolResultTool). Error mapping:
// ErrNotFound -> Found:false (same posture as the blob path, no reason
// leaked); ErrInvalidLocator / ErrUnsupportedStore / ErrSecurity ->
// BadRequest; everything else -> Internal.
func readSpillResult(ctx context.Context, store toolspill.Store, lg loggateway.Logger, input readToolResultInput) (readToolResultOutput, error) {
	loc, err := toolspill.ParseLocator(input.BlobID)
	if err != nil {
		return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "invalid spill locator: "+err.Error())
	}
	if store == nil {
		// Gray gate: locator parsed and validated, but this deployment has no
		// spill store wired yet (batch B). BadRequest — never a silent nil.
		return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "spill locator resolution is not wired in this deployment yet")
	}
	sessionID := sessionIDFromContext(ctx)
	rec, err := store.Get(ctx, sessionID, loc.String())
	if err != nil {
		switch {
		case errors.Is(err, toolspill.ErrNotFound):
			return readToolResultOutput{Found: false}, nil
		case errors.Is(err, toolspill.ErrInvalidLocator),
			errors.Is(err, toolspill.ErrUnsupportedStore),
			errors.Is(err, toolspill.ErrSecurity):
			return readToolResultOutput{}, apierror.BadRequest(apierror.DomainTool, "spill locator rejected: "+err.Error())
		default:
			return readToolResultOutput{}, apierror.Internal(apierror.DomainTool, "read spill: "+err.Error())
		}
	}

	lg.Info("spilled tool result read",
		loggateway.StepID("tool.read_tool_result.spill"),
		loggateway.SessionID(sessionID),
		loggateway.Str("locator", loc.String()),
		loggateway.Int64("bytes", rec.ByteSize),
	)

	limit := clampReadLimit(input.Limit)
	content, offset, totalSize, hasMore := sliceContent(string(rec.Content), input.Offset, limit)
	return readToolResultOutput{
		Content:   content,
		Found:     true,
		Offset:    offset,
		TotalSize: totalSize,
		HasMore:   hasMore,
	}, nil
}

// clampReadLimit applies the default and the hard cap to a requested limit.
func clampReadLimit(limit int) int {
	if limit <= 0 {
		return DefaultReadChunkSize
	}
	if limit > MaxReadChunkSize {
		return MaxReadChunkSize
	}
	return limit
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
