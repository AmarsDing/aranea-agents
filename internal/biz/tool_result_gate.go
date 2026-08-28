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
	// UserInputHardLimitChars 单条用户纯文本输入的硬上限（字符数）。
	// 超阈值（ToolResultSizeThreshold）走 blob 转存 + preview 注入；
	// 超硬上限直接拒绝——再大应走附件通道，而非粘贴进输入框。
	UserInputHardLimitChars = 200000
)

// 单轮超限治理：CheckUserInput 的 source 取值（写入 blob.ToolName，与工具结果区分）。
const (
	ToolResultSourceUserInput  = "user_input"
	ToolResultSourceAttachment = "attachment_text"
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

// CheckUserInput 对超阈值的用户输入/附件文本落地 blob，返回 preview。
// source 取 ToolResultSourceUserInput / ToolResultSourceAttachment，写入
// blob.ToolName 以与工具结果区分。幂等语义与 Check 一致（按 sessionID+messageID）。
// Stability:evolving
func (g *ToolResultGate) CheckUserInput(ctx context.Context, sessionID, messageID, source, fullContent string) (ToolResultGateResult, error) {
	return g.Check(ctx, sessionID, messageID, source, "", fullContent, 0)
}

// Archive persists fullContent as a blob UNCONDITIONALLY (no size gate) and
// records a replacement for cross-turn idempotency. It backs the tool-result
// prune hook (79-runtime-governance R2): the hook rewrites only the per-call
// request copy while the session store keeps originals, so the same content is
// re-scanned every turn — the replacement record (sessionID+messageID) makes
// re-archiving return the existing blob id instead of duplicating rows.
//
// PreviewText carries the prune pointer (not the gate's frozen preview); the
// gate's own BeforeModel hook never re-touches pruned content (far below
// ToolResultSizeThreshold), so the stored preview is only an idempotency
// artifact. BlobID is what the hook embeds in the pointer text.
//
// Stability:evolving
func (g *ToolResultGate) Archive(ctx context.Context, sessionID, messageID, toolName, fullContent string, turnNumber int) (ToolResultGateResult, error) {
	// Idempotency: an existing replacement (from a prior prune turn, or from
	// the size gate) pins the blob id for this message.
	existing, err := g.replacementReader.GetReplacementByMessage(ctx, sessionID, messageID)
	if err == nil && existing != nil {
		return ToolResultGateResult{
			BlobID:      existing.ResultBlobID,
			PreviewText: existing.PreviewText,
			DidPersist:  true,
		}, nil
	}

	byteSize := len(fullContent)
	blob := &ToolResultBlob{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		TurnNumber:       turnNumber,
		ToolName:         toolName,
		FullContent:      fullContent,
		ContentSizeChars: utf8.RuneCountInString(fullContent),
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	if err := g.blobWriter.SaveBlob(ctx, blob); err != nil {
		return ToolResultGateResult{}, apierror.Internal("TOOL_RESULT", "save blob: %v", err)
	}

	pointer := ToolResultPrunePointer(byteSize, toolName, blob.ID)
	rep := &ToolResultReplacement{
		ID:           uuid.NewString(),
		SessionID:    sessionID,
		MessageID:    messageID,
		ResultBlobID: blob.ID,
		PreviewText:  pointer,
		ReplacedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if err := g.replacementWriter.SaveReplacement(ctx, rep); err != nil {
		return ToolResultGateResult{}, apierror.Internal("TOOL_RESULT", "save replacement: %v", err)
	}

	return ToolResultGateResult{BlobID: blob.ID, PreviewText: pointer, DidPersist: true}, nil
}

// ToolResultPrunePointer renders the deterministic stub that replaces a pruned
// tool result in the request copy (79-runtime-governance design §3.2). Pure
// function of (byteSize, toolName, blobID) — stable across turns for a fixed
// blob id, so the post-prune prefix re-stabilizes after the first pruned turn.
func ToolResultPrunePointer(byteSize int, toolName, blobID string) string {
	return fmt.Sprintf("[已剪枝 tool_result｜原 %d 字节｜tool=%s｜blob=%s｜取回: 调用 read_tool_result(blob_id=%s)]", byteSize, toolName, blobID, blobID)
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
