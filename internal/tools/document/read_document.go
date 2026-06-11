package document

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	pdfpkg "github.com/ledongthuc/pdf"
	docreader "trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader"
	docxreader "trpc.group/trpc-go/trpc-agent-go/knowledge/document/reader/docx"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

const (
	toolReadDocument         = "read_document"
	defaultReadDocumentChars = 6000
)

type readDocumentInput struct {
	Path     string `json:"path"`
	Page     *int   `json:"page,omitempty"`
	MaxChars *int   `json:"max_chars,omitempty"`
}

type readDocumentOutput struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	PageCount int    `json:"page_count,omitempty"`
	Page      *int   `json:"page,omitempty"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
}

func NewReadDocumentTool(baseDir string) trpctool.Tool {
	return trpcfunction.NewFunctionTool(
		func(ctx context.Context, in readDocumentInput) (readDocumentOutput, error) {
			if strings.TrimSpace(in.Path) == "" {
				return readDocumentOutput{}, kerrors.BadRequest("READ_DOCUMENT_PATH_REQUIRED", "path is required")
			}

			path, err := ValidatePath(strings.TrimSpace(in.Path), baseDir)
			if err != nil {
				return readDocumentOutput{}, kerrors.BadRequest("READ_DOCUMENT_PATH_INVALID", err.Error())
			}

			kind := documentKindFromPath(path)
			if kind == "" {
				return readDocumentOutput{}, kerrors.BadRequest("READ_DOCUMENT_UNSUPPORTED_TYPE", fmt.Sprintf("unsupported document type: %s", filepath.Ext(path)))
			}

			if err := ValidateFileSize(path); err != nil {
				return readDocumentOutput{}, kerrors.BadRequest("READ_DOCUMENT_FILE_TOO_LARGE", err.Error())
			}

			info, err := os.Stat(path)
			if err != nil {
				return readDocumentOutput{}, kerrors.InternalServer("READ_DOCUMENT_STAT_ERROR", fmt.Sprintf("stat path: %v", err))
			}
			if info.IsDir() {
				return readDocumentOutput{}, kerrors.BadRequest("READ_DOCUMENT_PATH_IS_DIR", fmt.Sprintf("path is a directory: %s", path))
			}

			maxChars := defaultReadDocumentChars
			if in.MaxChars != nil && *in.MaxChars > 0 {
				maxChars = *in.MaxChars
			}

			text, pageCount, err := readDocumentText(path, kind, in.Page)
			if err != nil {
				return readDocumentOutput{}, err
			}

			text, truncated := truncateText(text, maxChars)

			return readDocumentOutput{
				Path:      path,
				Kind:      kind,
				Title:     filepath.Base(path),
				PageCount: pageCount,
				Page:      normalizedPositive(in.Page),
				Text:      text,
				Truncated: truncated,
			}, nil
		},
		trpcfunction.WithName(toolReadDocument),
		trpcfunction.WithDescription(
			"Read a document from a local path. "+
				"Supports PDF, DOCX, and plain text files. "+
				"Use this instead of exec_command to inspect documents.",
		),
		trpcfunction.WithInputSchema(&trpctool.Schema{
			Type:     "object",
			Required: []string{"path"},
			Properties: map[string]*trpctool.Schema{
				"path": {
					Type:        "string",
					Description: "Document file path.",
				},
				"page": {
					Type:        "integer",
					Description: "Optional 1-based PDF page number.",
				},
				"max_chars": {
					Type:        "integer",
					Description: "Optional maximum characters to return.",
				},
			},
		}),
		trpcfunction.WithOutputSchema(&trpctool.Schema{
			Type:     "object",
			Required: []string{"path", "kind", "text"},
			Properties: map[string]*trpctool.Schema{
				"path":       {Type: "string", Description: "Resolved file path."},
				"kind":       {Type: "string", Description: "Document kind (pdf, docx, text)."},
				"title":      {Type: "string", Description: "File name."},
				"page_count": {Type: "integer", Description: "Total pages (PDF only)."},
				"page":       {Type: "integer", Description: "Selected page (PDF only)."},
				"text":       {Type: "string", Description: "Extracted text content."},
				"truncated":  {Type: "boolean", Description: "Whether output was truncated."},
			},
		}),
	)
}

func documentKindFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "pdf"
	case ".docx", ".doc":
		return "docx"
	case ".txt", ".md", ".markdown", ".json", ".yaml", ".yml", ".log", ".csv":
		return "text"
	default:
		return ""
	}
}

func readDocumentText(path string, kind string, page *int) (string, int, error) {
	switch kind {
	case "pdf":
		return readPDFText(path, page)
	case "docx":
		if normalizedPositive(page) != nil {
			return "", 0, kerrors.BadRequest("READ_DOCUMENT_PAGE_UNSUPPORTED", "page is only supported for PDF files")
		}
		text, err := readDOCXText(path)
		return text, 0, err
	case "text":
		if normalizedPositive(page) != nil {
			return "", 0, kerrors.BadRequest("READ_DOCUMENT_PAGE_UNSUPPORTED", "page is only supported for PDF files")
		}
		text, err := readTextFile(path)
		return text, 0, err
	default:
		return "", 0, kerrors.BadRequest("READ_DOCUMENT_UNSUPPORTED_KIND", fmt.Sprintf("unsupported document kind: %s", kind))
	}
}

func readPDFText(path string, page *int) (string, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, kerrors.InternalServer("READ_DOCUMENT_OPEN_PDF_ERROR", fmt.Sprintf("open pdf: %v", err))
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", 0, kerrors.InternalServer("READ_DOCUMENT_STAT_PDF_ERROR", fmt.Sprintf("stat pdf: %v", err))
	}

	reader, err := pdfpkg.NewReader(file, info.Size())
	if err != nil {
		return "", 0, kerrors.InternalServer("READ_DOCUMENT_READ_PDF_ERROR", fmt.Sprintf("read pdf: %v", err))
	}

	pageCount := reader.NumPage()
	selectedPage := normalizedPositive(page)
	if selectedPage != nil {
		if *selectedPage > pageCount {
			return "", 0, kerrors.BadRequest("READ_DOCUMENT_PAGE_EXCEEDS", fmt.Sprintf("page %d exceeds page count %d", *selectedPage, pageCount))
		}
		return pdfPageText(reader, *selectedPage), pageCount, nil
	}

	var builder strings.Builder
	for pageIndex := 1; pageIndex <= pageCount; pageIndex++ {
		text := pdfPageText(reader, pageIndex)
		if strings.TrimSpace(text) == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(text)
	}
	return builder.String(), pageCount, nil
}

func pdfPageText(reader *pdfpkg.Reader, pageIndex int) string {
	if reader == nil {
		return ""
	}
	page := reader.Page(pageIndex)
	if page.V.IsNull() {
		return ""
	}
	text, err := page.GetPlainText(nil)
	if err != nil {
		// PDF page extraction errors are common (scanned pages, encrypted content).
		// Return empty string rather than failing the entire document read.
		return ""
	}
	return text
}

func readDOCXText(path string) (string, error) {
	rdr := docxreader.New(docreader.WithChunk(false))
	docs, err := rdr.ReadFromFile(path)
	if err != nil {
		return "", kerrors.InternalServer("READ_DOCUMENT_READ_DOCX_ERROR", fmt.Sprintf("read docx: %v", err))
	}

	parts := make([]string, 0, len(docs))
	for _, doc := range docs {
		if doc == nil {
			continue
		}
		text := strings.TrimSpace(doc.Content)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n"), nil
}

func readTextFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", kerrors.InternalServer("READ_DOCUMENT_OPEN_FILE_ERROR", fmt.Sprintf("read file: %v", err))
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFileSize))
	if err != nil {
		return "", kerrors.InternalServer("READ_DOCUMENT_READ_FILE_ERROR", fmt.Sprintf("read file: %v", err))
	}
	return string(data), nil
}
