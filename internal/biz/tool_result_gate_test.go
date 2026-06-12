package biz

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

type stubBlobReader struct {
	blob *ToolResultBlob
}

func (s *stubBlobReader) GetBlob(_ context.Context, id string) (*ToolResultBlob, error) {
	if s.blob != nil && s.blob.ID == id {
		return s.blob, nil
	}
	return nil, nil
}
func (s *stubBlobReader) ListBlobsBySession(context.Context, string, int) ([]*ToolResultBlob, error) {
	return nil, nil
}

type stubBlobWriter struct {
	saved *ToolResultBlob
	err   error
}

func (s *stubBlobWriter) SaveBlob(_ context.Context, blob *ToolResultBlob) error {
	s.saved = blob
	return s.err
}

type stubReplacementReader struct {
	result *ToolResultReplacement
	err    error
}

func (s *stubReplacementReader) GetReplacementByMessage(_ context.Context, _, _ string) (*ToolResultReplacement, error) {
	return s.result, s.err
}

type stubReplacementWriter struct {
	saved *ToolResultReplacement
	err   error
}

func (s *stubReplacementWriter) SaveReplacement(_ context.Context, r *ToolResultReplacement) error {
	s.saved = r
	return s.err
}

func makeLongContent(n int) string {
	return strings.Repeat("x", n)
}

func TestToolResultGate_Check_BelowThreshold(t *testing.T) {
	gate := NewToolResultGate(&stubBlobReader{}, &stubBlobWriter{}, &stubReplacementReader{}, &stubReplacementWriter{})
	result, err := gate.Check(context.Background(), "sess1", "msg1", "tool1", "", "short content", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DidPersist {
		t.Fatal("should not persist below threshold")
	}
	if result.BlobID != "" {
		t.Fatal("BlobID should be empty")
	}
}

func TestToolResultGate_Check_AboveThreshold(t *testing.T) {
	bw := &stubBlobWriter{}
	rw := &stubReplacementWriter{}
	gate := NewToolResultGate(&stubBlobReader{}, bw, &stubReplacementReader{}, rw)

	longContent := makeLongContent(ToolResultSizeThreshold + 1)
	result, err := gate.Check(context.Background(), "sess1", "msg1", "read_file", "path=/foo", longContent, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.DidPersist {
		t.Fatal("should persist above threshold")
	}
	if result.BlobID == "" {
		t.Fatal("BlobID should not be empty")
	}
	if result.PreviewText == "" {
		t.Fatal("PreviewText should not be empty")
	}
	if bw.saved == nil {
		t.Fatal("blob should be saved")
	}
	if bw.saved.SessionID != "sess1" {
		t.Fatalf("sessionID = %q, want sess1", bw.saved.SessionID)
	}
	if bw.saved.TurnNumber != 3 {
		t.Fatalf("turnNumber = %d, want 3", bw.saved.TurnNumber)
	}
	if bw.saved.ToolName != "read_file" {
		t.Fatalf("toolName = %q, want read_file", bw.saved.ToolName)
	}
	if bw.saved.ContentSizeChars != len(longContent) {
		t.Fatalf("contentSizeChars = %d, want %d", bw.saved.ContentSizeChars, len(longContent))
	}
	if rw.saved == nil {
		t.Fatal("replacement should be saved")
	}
	if rw.saved.ResultBlobID != result.BlobID {
		t.Fatalf("replacement blobID = %q, want %q", rw.saved.ResultBlobID, result.BlobID)
	}
}

func TestToolResultGate_Check_BlobSaveError(t *testing.T) {
	bw := &stubBlobWriter{err: ErrNotFound}
	gate := NewToolResultGate(&stubBlobReader{}, bw, &stubReplacementReader{}, &stubReplacementWriter{})

	_, err := gate.Check(context.Background(), "sess1", "msg1", "tool1", "", makeLongContent(ToolResultSizeThreshold+1), 1)
	if err == nil {
		t.Fatal("should return error when blob save fails")
	}
}

func TestToolResultGate_Check_ReplacementSaveError(t *testing.T) {
	rw := &stubReplacementWriter{err: ErrNotFound}
	gate := NewToolResultGate(&stubBlobReader{}, &stubBlobWriter{}, &stubReplacementReader{}, rw)

	_, err := gate.Check(context.Background(), "sess1", "msg1", "tool1", "", makeLongContent(ToolResultSizeThreshold+1), 1)
	if err == nil {
		t.Fatal("should return error when replacement save fails")
	}
}

func TestToolResultGate_BlobReader(t *testing.T) {
	br := &stubBlobReader{}
	gate := NewToolResultGate(br, &stubBlobWriter{}, &stubReplacementReader{}, &stubReplacementWriter{})
	if gate.BlobReader() != br {
		t.Fatal("BlobReader should return the injected reader")
	}
}

func TestToolResultGate_Check_Idempotent(t *testing.T) {
	// If a replacement already exists for (sessionID, messageID), Check should
	// return the existing result without creating new blob/replacement records.
	existing := &ToolResultReplacement{
		ID:           "existing-rep-id",
		SessionID:    "sess1",
		MessageID:    "msg1",
		ResultBlobID: "existing-blob-id",
		PreviewText:  "existing preview",
	}
	rr := &stubReplacementReader{result: existing}
	bw := &stubBlobWriter{}
	rw := &stubReplacementWriter{}
	gate := NewToolResultGate(&stubBlobReader{}, bw, rr, rw)

	result, err := gate.Check(context.Background(), "sess1", "msg1", "tool1", "", makeLongContent(ToolResultSizeThreshold+1), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BlobID != "existing-blob-id" {
		t.Fatalf("BlobID = %q, want existing-blob-id", result.BlobID)
	}
	if result.PreviewText != "existing preview" {
		t.Fatalf("PreviewText = %q, want existing preview", result.PreviewText)
	}
	if !result.DidPersist {
		t.Fatal("DidPersist should be true for existing replacement")
	}
	if bw.saved != nil {
		t.Fatal("should not save new blob when replacement already exists")
	}
	if rw.saved != nil {
		t.Fatal("should not save new replacement when replacement already exists")
	}
}

func TestFreezePreview_ShortContent(t *testing.T) {
	content := "short"
	result := freezePreview(content, "blob1")
	if result != "short" {
		t.Fatalf("short content should not be truncated, got %q", result)
	}
}

func TestFreezePreview_LongContent(t *testing.T) {
	content := makeLongContent(ToolResultPreviewSize + 100)
	result := freezePreview(content, "blob-abc")
	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}
	if !strings.Contains(result, "blob-abc") {
		t.Fatal("preview should contain blob_id reference")
	}
	if !strings.Contains(result, "truncated") {
		t.Fatal("preview should indicate truncation")
	}
	head := result[:ToolResultPreviewSize]
	if head != content[:ToolResultPreviewSize] {
		t.Fatal("preview head should match content head")
	}
}

func TestFreezePreview_ExactlyAtThreshold(t *testing.T) {
	content := makeLongContent(ToolResultPreviewSize)
	result := freezePreview(content, "blob1")
	if result != content {
		t.Fatal("content exactly at preview size should not be truncated")
	}
}

func TestFreezePreview_MultibyteUTF8(t *testing.T) {
	// Each CJK character is 3 bytes in UTF-8. Create content where the
	// byte-length exceeds ToolResultPreviewSize but rune count is exactly
	// at the threshold, and verify no invalid UTF-8 is produced.
	cjkChar := "中" // 3 bytes per rune
	content := strings.Repeat(cjkChar, ToolResultPreviewSize)
	result := freezePreview(content, "blob-cjk")
	// Content is exactly at preview size in runes → should not be truncated.
	if result != content {
		t.Fatal("content at preview size (in runes) should not be truncated")
	}

	// Now exceed the threshold by one rune.
	contentLong := content + cjkChar
	resultLong := freezePreview(contentLong, "blob-cjk-long")
	if !strings.Contains(resultLong, "truncated") {
		t.Fatal("long content should indicate truncation")
	}
	// Verify the preview head is valid UTF-8.
	headRunes := []rune(resultLong)
	if len(headRunes) < ToolResultPreviewSize {
		t.Fatalf("preview head should have at least %d runes, got %d", ToolResultPreviewSize, len(headRunes))
	}
	// Verify no invalid UTF-8 by round-tripping.
	if !utf8.ValidString(resultLong) {
		t.Fatal("preview should be valid UTF-8")
	}
}

func TestFreezePreview_PaginationHint(t *testing.T) {
	content := makeLongContent(ToolResultSizeThreshold + 1)
	result := freezePreview(content, "blob-pag")
	if !strings.Contains(result, "read_tool_result") {
		t.Fatal("preview should mention read_tool_result for pagination")
	}
	if !strings.Contains(result, "offset=") {
		t.Fatal("preview should mention offset parameter")
	}
}
