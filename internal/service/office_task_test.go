package service

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	v1 "aranea-agents/api/kratos/office/v1"
)

func TestSafeDownloadType(t *testing.T) {
	for in, want := range map[string]string{
		"image/png":                "image/png",
		"application/pdf":          "application/pdf",
		"text/plain; charset=utf8": "text/plain; charset=utf8",
		"":                         "application/octet-stream",
		"text/html":                "application/octet-stream",
		"Text/HTML; charset=utf-8": "application/octet-stream",
		"image/svg+xml":            "application/octet-stream",
		"application/xhtml+xml":    "application/octet-stream",
		"text/xml":                 "application/octet-stream",
		"application/javascript":   "application/octet-stream",
	} {
		if got := safeDownloadType(in); got != want {
			t.Errorf("safeDownloadType(%q) = %q, want %q", in, got, want)
		}
	}
}

// 任务附件走 multipart：文件名、类型、内容都转进请求消息；task_id 留给路径绑定。
func TestDecodeOfficeTaskFileMultipart(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "报价.xlsx")
	_, _ = part.Write([]byte("PK\x03\x04data"))
	_ = w.WriteField("filename", "季度报价.xlsx")
	_ = w.Close()

	r := httptest.NewRequest("POST", "/v1/office/tasks/otk_1/files", &body)
	r.Header.Set("Content-Type", w.FormDataContentType())
	var req v1.UploadOfficeTaskFileRequest
	if err := DecodeSkillImportRequest(r, &req); err != nil {
		t.Fatal(err)
	}
	if req.GetFilename() != "季度报价.xlsx" || string(req.GetContent()) != "PK\x03\x04data" || req.GetContentType() != "application/octet-stream" {
		t.Fatalf("decoded = %q %q %q", req.GetFilename(), req.GetContent(), req.GetContentType())
	}

	// 不是 multipart 的照旧走 JSON
	r = httptest.NewRequest("POST", "/v1/office/tasks/otk_1/files", bytes.NewBufferString(`{"filename":"a.txt","content":"aGk="}`))
	r.Header.Set("Content-Type", "application/json")
	req.Reset()
	if err := DecodeSkillImportRequest(r, &req); err != nil {
		t.Fatal(err)
	}
	if req.GetFilename() != "a.txt" || string(req.GetContent()) != "hi" {
		t.Fatalf("json decoded = %q %q", req.GetFilename(), req.GetContent())
	}
}
