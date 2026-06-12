package biz

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
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
	replacementReader ToolResultReplacementReader
	replacementWriter ToolResultReplacementWriter
}

func NewToolResultGate(
	blobReader ToolResultBlobReader,
	blobWriter ToolResultBlobWriter,
	replacementReader ToolResultReplacementReader,
	replacementWriter ToolResultReplacementWriter,
) *ToolResultGate {
	return &ToolResultGate{
		blobReader:        blobReader,
		blobWriter:        blobWriter,
		replacementReader: replacementReader,
		replacementWriter: replacementWriter,
	}
}

func (g *ToolResultGate) BlobReader() ToolResultBlobReader {
	return g.blobReader
}

func (g *ToolResultGate) Check(ctx context.Context, sessionID, messageID, toolName, toolArgsSummary, fullContent string, turnNumber int) (ToolResultGateResult, error) {
	runeCount := utf8.RuneCountInString(fullContent)
	if runeCount <= ToolResultSizeThreshold {
		return ToolResultGateResult{}, nil
	}

	// Idempotency: if a replacement already exists for this (sessionID, messageID),
	// return the existing result instead of creating duplicates.
	existing, err := g.replacementReader.GetReplacementByMessage(ctx, sessionID, messageID)
	if err == nil && existing != nil {
		return ToolResultGateResult{
			BlobID:      existing.ResultBlobID,
			PreviewText: existing.PreviewText,
			DidPersist:  true,
		}, nil
	}

	blob := &ToolResultBlob{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		TurnNumber:       turnNumber,
		ToolName:         toolName,
		ToolArgsSummary:  toolArgsSummary,
		FullContent:      fullContent,
		ContentSizeChars: runeCount,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}

	if err := g.blobWriter.SaveBlob(ctx, blob); err != nil {
		return ToolResultGateResult{}, apierror.Internal("TOOL_RESULT", "save blob: %v", err)
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
		return ToolResultGateResult{}, apierror.Internal("TOOL_RESULT", "save replacement: %v", err)
	}

	return ToolResultGateResult{
		BlobID:      blob.ID,
		PreviewText: preview,
		DidPersist:  true,
	}, nil
}

func freezePreview(fullContent, blobID string) string {
	runeCount := utf8.RuneCountInString(fullContent)
	if runeCount <= ToolResultPreviewSize {
		return fullContent
	}
	runes := []rune(fullContent)
	head := string(runes[:ToolResultPreviewSize])
	truncated := fmt.Sprintf(
		"\n\n... [truncated %d → %d chars, blob_id=%s. Use read_tool_result with blob_id, offset=%d to read more] ...",
		runeCount, ToolResultPreviewSize, blobID, ToolResultPreviewSize,
	)
	return head + truncated
}
