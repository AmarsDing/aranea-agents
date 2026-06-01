package biz

import (
	"context"
	"strings"
	"testing"
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
	gate := NewToolResultGate(&stubBlobReader{}, &stubBlobWriter{}, &stubReplacementWriter{})
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
	gate := NewToolResultGate(&stubBlobReader{}, bw, rw)

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
	gate := NewToolResultGate(&stubBlobReader{}, bw, &stubReplacementWriter{})

	_, err := gate.Check(context.Background(), "sess1", "msg1", "tool1", "", makeLongContent(ToolResultSizeThreshold+1), 1)
	if err == nil {
		t.Fatal("should return error when blob save fails")
	}
}

func TestToolResultGate_Check_ReplacementSaveError(t *testing.T) {
	rw := &stubReplacementWriter{err: ErrNotFound}
	gate := NewToolResultGate(&stubBlobReader{}, &stubBlobWriter{}, rw)

	_, err := gate.Check(context.Background(), "sess1", "msg1", "tool1", "", makeLongContent(ToolResultSizeThreshold+1), 1)
	if err == nil {
		t.Fatal("should return error when replacement save fails")
	}
}

func TestToolResultGate_BlobReader(t *testing.T) {
	br := &stubBlobReader{}
	gate := NewToolResultGate(br, &stubBlobWriter{}, &stubReplacementWriter{})
	if gate.BlobReader() != br {
		t.Fatal("BlobReader should return the injected reader")
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
