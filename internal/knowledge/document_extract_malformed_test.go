package knowledge

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
)

// 残缺 docx（只有 word/document.xml，缺 _rels）曾让 docxlib 空指针 panic，
// 把本机资料库的同步循环整个带走；现在应当返回解析错误。
func TestExtractOfficeText_MalformedDocxReturnsError(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body/></w:document>`))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	text, err := extractOfficeText(context.Background(), buf.Bytes(), "坏合同.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err == nil {
		t.Fatalf("want error for malformed docx, got text %q", text)
	}
}
