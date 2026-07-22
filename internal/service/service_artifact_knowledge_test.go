package service_test

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoArtifactMeta(t *testing.T) {
	tests := []struct {
		name string
		in   biz.Artifact
	}{
		{
			name: "full",
			in: biz.Artifact{
				ID: "a1", SessionID: "s1", Name: "file.txt",
				MimeType: "text/plain", Size: 1024, SHA256: "abc123",
				StorageKind: "sqlite", StorageURI: "db://a1",
				Version: 3, CreatedAt: "2024-01-01",
			},
		},
		{name: "zero", in: biz.Artifact{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoArtifactMeta(tt.in)
			if got.Id != tt.in.ID {
				t.Errorf("Id = %q, want %q", got.Id, tt.in.ID)
			}
			if got.SessionId != tt.in.SessionID {
				t.Errorf("SessionId = %q, want %q", got.SessionId, tt.in.SessionID)
			}
			if got.Name != tt.in.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.in.Name)
			}
			if got.MimeType != tt.in.MimeType {
				t.Errorf("MimeType = %q, want %q", got.MimeType, tt.in.MimeType)
			}
			if got.Size != tt.in.Size {
				t.Errorf("Size = %d, want %d", got.Size, tt.in.Size)
			}
			if got.Sha256 != tt.in.SHA256 {
				t.Errorf("Sha256 = %q, want %q", got.Sha256, tt.in.SHA256)
			}
			if got.StorageKind != tt.in.StorageKind {
				t.Errorf("StorageKind = %q, want %q", got.StorageKind, tt.in.StorageKind)
			}
			if got.Version != int32(tt.in.Version) {
				t.Errorf("Version = %d, want %d", got.Version, int32(tt.in.Version))
			}
		})
	}
}

func TestToProtoCollection(t *testing.T) {
	in := biz.KnowledgeCollection{
		ID: "col1", Name: "docs", Description: "doc collection",
		EmbeddingModel: "text-embedding-3-small", Dim: 1536,
		Status: "active", DocumentCount: 10, ChunkCount: 100,
		Workspace: "ws1", CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
	}
	got := service.ToProtoCollection(in)
	if got.Id != "col1" {
		t.Errorf("Id = %q, want %q", got.Id, "col1")
	}
	if got.Name != "docs" {
		t.Errorf("Name = %q, want %q", got.Name, "docs")
	}
	if got.EmbeddingModel != "text-embedding-3-small" {
		t.Errorf("EmbeddingModel = %q, want %q", got.EmbeddingModel, "text-embedding-3-small")
	}
	if got.Dim != 1536 {
		t.Errorf("Dim = %d, want 1536", got.Dim)
	}
	if got.DocumentCount != 10 {
		t.Errorf("DocumentCount = %d, want 10", got.DocumentCount)
	}
	if got.ChunkCount != 100 {
		t.Errorf("ChunkCount = %d, want 100", got.ChunkCount)
	}
}

func TestToProtoDocument(t *testing.T) {
	tests := []struct {
		name            string
		in              biz.KnowledgeDocument
		wantExtractSupp bool
	}{
		{
			name: "pdf_extract_supported",
			in: biz.KnowledgeDocument{
				ID: "doc1", CollectionID: "col1", Source: "upload",
				MimeType: "application/pdf", SizeBytes: 2048,
				ChunkCount: 5, Status: "indexed", ErrorMessage: "",
				CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
			},
			wantExtractSupp: true,
		},
		{
			name: "unsupported_mime",
			in: biz.KnowledgeDocument{
				ID: "doc2", CollectionID: "col1",
				MimeType: "application/x-binary",
			},
			wantExtractSupp: false,
		},
		{
			name: "text_custom_extract_supported",
			in: biz.KnowledgeDocument{
				ID: "doc3", CollectionID: "col1",
				MimeType: "text/x-custom",
			},
			wantExtractSupp: true,
		},
		{
			name: "docx_extract_supported",
			in: biz.KnowledgeDocument{
				ID: "doc4", CollectionID: "col1",
				MimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			},
			wantExtractSupp: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoDocument(tt.in)
			if got.Id != tt.in.ID {
				t.Errorf("Id = %q, want %q", got.Id, tt.in.ID)
			}
			if got.CollectionId != tt.in.CollectionID {
				t.Errorf("CollectionId = %q, want %q", got.CollectionId, tt.in.CollectionID)
			}
			if got.ExtractSupported != tt.wantExtractSupp {
				t.Errorf("ExtractSupported = %v, want %v", got.ExtractSupported, tt.wantExtractSupp)
			}
		})
	}
}

func TestIsExtractSupported(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"text_plain", "text/plain", true},
		{"text_markdown", "text/markdown", true},
		{"text_csv", "text/csv", true},
		{"text_html", "text/html", true},
		{"text_xml", "text/xml", true},
		{"application_json", "application/json", true},
		{"application_xml", "application/xml", true},
		{"application_pdf", "application/pdf", true},
		{"application_msword", "application/msword", true},
		{"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true},
		{"pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", true},
		{"text_custom", "text/x-custom", true},
		{"image_png", "image/png", false},
		{"application_octet", "application/octet-stream", false},
		{"empty", "", false},
		{"TEXT_PLAIN_upper", "TEXT/PLAIN", true},
		{"  text/plain  ", "  text/plain  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.IsExtractSupported(tt.in)
			if got != tt.want {
				t.Errorf("IsExtractSupported(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsAllowedIngestMIME(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"text_plain", "text/plain", true},
		{"text_markdown", "text/markdown", true},
		{"text_csv", "text/csv", true},
		{"application_json", "application/json", true},
		{"application_pdf", "application/pdf", true},
		{"image_png", "image/png", true},   // Phase 9：VisionExtractor 入库
		{"image_jpeg", "image/jpeg", true}, // Phase 9：VisionExtractor 入库
		{"image_webp", "image/webp", true}, // Phase 9：VisionExtractor 入库
		{"image_gif", "image/gif", false},  // 未支持格式仍拒绝
		{"application_msword", "application/msword", true},
		{"docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"text_custom_prefix", "text/x-unknown", true},
		{"application_octet", "application/octet-stream", false},
		{"video_mp4", "video/mp4", false},
		{"audio_mp3", "audio/mpeg", false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.IsAllowedIngestMIME(tt.in)
			if got != tt.want {
				t.Errorf("IsAllowedIngestMIME(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestToProtoArtifactMeta_FieldMapping(t *testing.T) {
	in := biz.Artifact{
		ID: "fid", SessionID: "sid", Name: "fname",
		MimeType: "application/json", Size: 2048,
		SHA256: "sha", StorageKind: "fs", StorageURI: "/tmp/f",
		Version: 5, CreatedAt: "2024-06-01",
	}
	got := service.ToProtoArtifactMeta(in)
	wantFields := map[string]interface{}{
		"Id": got.Id, "SessionId": got.SessionId, "Name": got.Name,
		"MimeType": got.MimeType, "Size": got.Size, "Sha256": got.Sha256,
		"StorageKind": got.StorageKind, "StorageUri": got.StorageUri,
		"Version": got.Version, "CreatedAt": got.CreatedAt,
	}
	expected := map[string]interface{}{
		"Id": "fid", "SessionId": "sid", "Name": "fname",
		"MimeType": "application/json", "Size": int64(2048), "Sha256": "sha",
		"StorageKind": "fs", "StorageUri": "/tmp/f",
		"Version": int32(5), "CreatedAt": "2024-06-01",
	}
	for k := range expected {
		if wantFields[k] != expected[k] {
			t.Errorf("field %s = %v, want %v", k, wantFields[k], expected[k])
		}
	}
}

func TestToProtoDocument_ExtractSupportedIntegration(t *testing.T) {
	mimeTypes := []struct {
		mime   string
		expect bool
	}{
		{"text/plain", true},
		{"application/pdf", true},
		{"image/png", false},
		{"application/octet-stream", false},
		{"text/x-rst", true},
	}
	for _, mt := range mimeTypes {
		doc := biz.KnowledgeDocument{ID: "d", MimeType: mt.mime}
		got := service.ToProtoDocument(doc)
		if got.ExtractSupported != mt.expect {
			t.Errorf("ToProtoDocument(mime=%q).ExtractSupported = %v, want %v", mt.mime, got.ExtractSupported, mt.expect)
		}
	}
}
