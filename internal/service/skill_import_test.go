package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/skill/v1"
	"aranea-agents/internal/service"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/middleware"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

func newImportSkillService(t *testing.T) *service.SkillService {
	t.Helper()
	eng := importer.NewEngine(nil, nil, nil, nil, loggateway.NewNoop())
	return service.NewSkillService(service.SkillServiceDeps{
		Import: eng,
		Lg:     loggateway.NewNoop(),
	})
}

func TestImportSkillZip_RequiresAuth(t *testing.T) {
	svc := newImportSkillService(t)
	_, err := svc.ImportSkillZip(context.Background(), &v1.ImportSkillZipRequest{
		File: []byte("x"), Filename: "x.zip",
	})
	if err != auth.ErrUnauthorized {
		t.Fatalf("unauthenticated: got %v, want ErrUnauthorized", err)
	}

	userCtx := auth.NewContext(context.Background(), &auth.Auth{UserID: 2, Access: "user"})
	_, err = svc.ImportSkillZip(userCtx, &v1.ImportSkillZipRequest{
		File: []byte("x"), Filename: "x.zip",
	})
	if err != auth.ErrForbidden {
		t.Fatalf("non-admin: got %v, want ErrForbidden", err)
	}
}

func TestImportSkillZip_MissingFile(t *testing.T) {
	svc := newImportSkillService(t)
	_, err := svc.ImportSkillZip(adminCtx(), &v1.ImportSkillZipRequest{})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("empty request: got %v, want BadRequest", err)
	}
}

func TestImportSkillZip_EngineNotConfigured(t *testing.T) {
	svc := service.NewSkillService(service.SkillServiceDeps{Lg: loggateway.NewNoop()})
	_, err := svc.ImportSkillZip(adminCtx(), &v1.ImportSkillZipRequest{
		File: []byte("x"), Filename: "x.zip",
	})
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("nil engine: got %v, want Internal", err)
	}
}

func TestImportSkillZip_RejectsNonZip(t *testing.T) {
	svc := newImportSkillService(t)
	_, err := svc.ImportSkillZip(adminCtx(), &v1.ImportSkillZipRequest{
		File: []byte("not-a-zip"), Filename: "skill.txt",
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("non-zip: got %v, want BadRequest", err)
	}
}

func TestImportSkillZip_ReturnsJobID(t *testing.T) {
	svc := newImportSkillService(t)
	out, err := svc.ImportSkillZip(adminCtx(), &v1.ImportSkillZipRequest{
		File: []byte("not-a-zip"), Filename: "skill.zip",
	})
	if err != nil {
		t.Fatalf("ImportSkillZip: %v", err)
	}
	if out.GetJobId() == "" {
		t.Fatal("expected job_id")
	}
}

func TestDecodeSkillImportRequest_Multipart(t *testing.T) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "pack.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("zip-bytes")); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/v1/skills/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	var req v1.ImportSkillZipRequest
	if err := service.DecodeSkillImportRequest(r, &req); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if req.GetFilename() != "pack.zip" || string(req.GetFile()) != "zip-bytes" {
		t.Fatalf("decoded filename=%q file=%q", req.GetFilename(), string(req.GetFile()))
	}
}

func TestImportSkillZipHTTP_JSONAndMultipart(t *testing.T) {
	svc := newImportSkillService(t)
	httpSrv := kratoshttp.NewServer(
		kratoshttp.RequestDecoder(service.DecodeSkillImportRequest),
		kratoshttp.Middleware(func(h middleware.Handler) middleware.Handler {
			return func(ctx context.Context, req any) (any, error) {
				return h(auth.NewContext(ctx, &auth.Auth{UserID: 1, Access: "admin"}), req)
			}
		}),
	)
	v1.RegisterSkillServiceHTTPServer(httpSrv, svc)
	ts := httptest.NewServer(httpSrv)
	defer ts.Close()

	t.Run("json", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{
			"file":     []byte("not-a-zip"),
			"filename": "skill.zip",
		})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(ts.URL+"/v1/skills/import", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
		}
		var out struct {
			JobID string `json:"jobId"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(out.JobID) == "" {
			t.Fatalf("missing jobId in %s", raw)
		}
	})

	t.Run("multipart", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, err := mw.CreateFormFile("file", "skill.zip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, "not-a-zip"); err != nil {
			t.Fatal(err)
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(ts.URL+"/v1/skills/import", mw.FormDataContentType(), &buf)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
		}
		if !bytes.Contains(raw, []byte("jobId")) && !bytes.Contains(raw, []byte("job_id")) {
			t.Fatalf("missing job id in %s", raw)
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		plain := kratoshttp.NewServer(kratoshttp.RequestDecoder(service.DecodeSkillImportRequest))
		v1.RegisterSkillServiceHTTPServer(plain, svc)
		plainTS := httptest.NewServer(plain)
		defer plainTS.Close()
		body, _ := json.Marshal(map[string]any{"file": []byte("x"), "filename": "x.zip"})
		resp, err := http.Post(plainTS.URL+"/v1/skills/import", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			raw, _ := io.ReadAll(resp.Body)
			t.Fatalf("status=%d body=%s, want 401", resp.StatusCode, raw)
		}
	})
}
