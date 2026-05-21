package knowledge

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	trpcdoc "trpc.group/trpc-go/trpc-agent-go/knowledge/document"
	"trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader"
)

// ExtractDocumentText decodes uploaded bytes into plain text for chunking.
// Binary formats (PDF, DOCX, …) use trpc-agent-go document readers.
func ExtractDocumentText(raw []byte, source, mimeType string) (string, error) {
	if len(raw) == 0 {
		return "", fmt.Errorf("document content is empty")
	}
	ext := resolveDocumentExtension(source, mimeType)
	if ext == ".html" || ext == ".htm" {
		return stripHTML(string(raw)), nil
	}
	if isPlainTextExtension(ext) && utf8.Valid(raw) {
		return string(raw), nil
	}
	r, ok := reader.GetReader(ext, reader.WithChunk(false))
	if !ok {
		if utf8.Valid(raw) {
			return string(raw), nil
		}
		return "", fmt.Errorf("unsupported document type %q (mime=%q)", ext, mimeType)
	}
	name := strings.TrimSpace(source)
	if name == "" {
		name = "upload" + ext
	}
	docs, err := r.ReadFromReader(name, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("read document: %w", err)
	}
	return joinDocumentTexts(docs), nil
}

func resolveDocumentExtension(source, mimeType string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(source)))
	if ext != "" {
		return ext
	}
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "application/pdf":
		return ".pdf"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case "application/msword":
		return ".doc"
	case "text/html":
		return ".html"
	case "text/markdown":
		return ".md"
	case "application/json":
		return ".json"
	case "text/plain":
		return ".txt"
	default:
		return ".txt"
	}
}

func isPlainTextExtension(ext string) bool {
	switch ext {
	case ".txt", ".text", ".md", ".markdown", ".json", ".csv", ".html", ".htm":
		return true
	default:
		return false
	}
}

func joinDocumentTexts(docs []*trpcdoc.Document) string {
	if len(docs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, d := range docs {
		if d == nil {
			continue
		}
		text := strings.TrimSpace(d.Content)
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}
	return b.String()
}
