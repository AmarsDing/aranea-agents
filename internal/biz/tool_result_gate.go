package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	ToolResultSizeThreshold = 50000
	ToolResultPreviewSize   = 2000
)

type ToolResultGateResult struct {
	BlobID      string
	PreviewText string
	DidPersist  bool
}

type ToolResultGate struct {
	blobReader        ToolResultBlobReader
	blobWriter        ToolResultBlobWriter
	replacementWriter ToolResultReplacementWriter
}

func NewToolResultGate(
	blobReader ToolResultBlobReader,
	blobWriter ToolResultBlobWriter,
	replacementWriter ToolResultReplacementWriter,
) *ToolResultGate {
	return &ToolResultGate{
		blobReader:        blobReader,
		blobWriter:        blobWriter,
		replacementWriter: replacementWriter,
	}
}

func (g *ToolResultGate) BlobReader() ToolResultBlobReader {
	return g.blobReader
}

func (g *ToolResultGate) Check(ctx context.Context, sessionID, messageID, toolName, toolArgsSummary, fullContent string, turnNumber int) (ToolResultGateResult, error) {
	if len(fullContent) <= ToolResultSizeThreshold {
		return ToolResultGateResult{}, nil
	}

	blob := &ToolResultBlob{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		TurnNumber:       turnNumber,
		ToolName:         toolName,
		ToolArgsSummary:  toolArgsSummary,
		FullContent:      fullContent,
		ContentSizeChars: len(fullContent),
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := g.blobWriter.SaveBlob(ctx, blob); err != nil {
		return ToolResultGateResult{}, kerrors.InternalServer("TOOL_RESULT", fmt.Sprintf("save blob: %v", err))
	}

	preview := freezePreview(fullContent, blob.ID)

	rep := &ToolResultReplacement{
		ID:           uuid.NewString(),
		SessionID:    sessionID,
		MessageID:    messageID,
		ResultBlobID: blob.ID,
		PreviewText:  preview,
		ReplacedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	if err := g.replacementWriter.SaveReplacement(ctx, rep); err != nil {
		return ToolResultGateResult{}, kerrors.InternalServer("TOOL_RESULT", fmt.Sprintf("save replacement: %v", err))
	}

	return ToolResultGateResult{
		BlobID:      blob.ID,
		PreviewText: preview,
		DidPersist:  true,
	}, nil
}

func freezePreview(fullContent, blobID string) string {
	head := fullContent
	if len(head) > ToolResultPreviewSize {
		head = head[:ToolResultPreviewSize]
	}
	truncated := ""
	if len(fullContent) > ToolResultPreviewSize {
		truncated = fmt.Sprintf("\n\n... [truncated %d → %d chars, blob_id=%s] ...", len(fullContent), ToolResultPreviewSize, blobID)
	}
	return head + truncated
}
