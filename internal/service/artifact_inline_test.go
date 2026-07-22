package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/artifact/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
)

// uploadArtifactWithMime uploads an artifact with an explicit MIME type.
func uploadArtifactWithMime(t *testing.T, svc *ArtifactService, ctx context.Context, sessionID, name, mime string) string {
	t.Helper()
	meta, err := svc.UploadArtifact(ctx, &v1.UploadArtifactRequest{
		SessionId:  sessionID,
		Name:       name,
		MimeType:   mime,
		DataBase64: base64.StdEncoding.EncodeToString([]byte("media-payload-0123456789")),
	})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	return meta.GetId()
}

// signedGet signs a download URL and issues the GET with the given extra query.
func signedGet(t *testing.T, svc *ArtifactService, ctx context.Context, id, extra string) *httptest.ResponseRecorder {
	t.Helper()
	signed, err := svc.SignDownloadUrl(ctx, &v1.SignDownloadUrlRequest{Id: id, Version: 1})
	if err != nil {
		t.Fatalf("SignDownloadUrl: %v", err)
	}
	rec := httptest.NewRecorder()
	svc.ServeSignedDownload(rec, httptest.NewRequest(http.MethodGet, signed.GetUrl()+extra, nil))
	return rec
}

// TestArtifactService_SignedDownload_Inline verifies the inline=1 parameter:
// allowlisted media types (audio/video/image) are served with
// Content-Disposition: inline so browsers can play them via <audio>/<video>;
// everything else stays attachment to prevent HTML/JS injection.
func TestArtifactService_SignedDownload_Inline(t *testing.T) {
	t.Setenv("DEPLOY_ENV", "dev")
	t.Setenv("KRATOS_ARTIFACT_SIGN_KEY", "")
	t.Setenv("KRATOS_AUTH_SECRET", "")

	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-ok": {ID: "sess-ok", WorkspaceID: "ws-ok"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)
	ctx := workspace.WithContext(context.Background(), "ws-ok")

	t.Run("video inline", func(t *testing.T) {
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "clip.mp4", "video/mp4")
		rec := signedGet(t, svc, ctx, id, "&inline=1")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
			t.Fatalf("expected inline disposition, got %q", cd)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "video/mp4" {
			t.Fatalf("expected video/mp4, got %q", ct)
		}
	})

	t.Run("audio inline", func(t *testing.T) {
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "voice.mp3", "audio/mpeg")
		rec := signedGet(t, svc, ctx, id, "&inline=1")
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "inline") {
			t.Fatalf("expected inline disposition, got %q", cd)
		}
	})

	t.Run("html stays attachment", func(t *testing.T) {
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "page.html", "text/html")
		rec := signedGet(t, svc, ctx, id, "&inline=1")
		if cd := rec.Header().Get("Content-Disposition"); strings.HasPrefix(cd, "inline") {
			t.Fatalf("text/html must not be inlined, got %q", cd)
		}
	})

	t.Run("default is attachment", func(t *testing.T) {
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "clip2.mp4", "video/mp4")
		rec := signedGet(t, svc, ctx, id, "")
		if cd := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
			t.Fatalf("expected attachment disposition, got %q", cd)
		}
	})

	t.Run("range request returns 206", func(t *testing.T) {
		id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", "clip3.mp4", "video/mp4")
		signed, err := svc.SignDownloadUrl(ctx, &v1.SignDownloadUrlRequest{Id: id, Version: 1})
		if err != nil {
			t.Fatalf("SignDownloadUrl: %v", err)
		}
		req := httptest.NewRequest(http.MethodGet, signed.GetUrl()+"&inline=1", nil)
		req.Header.Set("Range", "bytes=0-4")
		rec := httptest.NewRecorder()
		svc.ServeSignedDownload(rec, req)
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("expected 206, got %d", rec.Code)
		}
		if got := rec.Body.String(); got != "media" {
			t.Fatalf("expected first 5 bytes, got %q", got)
		}
	})
}

// TestArtifactService_PreviewArtifact_AudioVideoNoBase64 verifies audio/video
// previews report their kind without embedding base64 payloads.
func TestArtifactService_PreviewArtifact_AudioVideoNoBase64(t *testing.T) {
	lookup := &fakeSessionLookup{
		sessions: map[string]biz.Session{
			"sess-ok": {ID: "sess-ok", WorkspaceID: "ws-ok"},
		},
	}
	svc := newArtifactServiceWithLookup(lookup)
	ctx := workspace.WithContext(context.Background(), "ws-ok")

	for _, tc := range []struct {
		name string
		mime string
		want string
	}{
		{"audio", "audio/mpeg", "audio"},
		{"video", "video/mp4", "video"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := uploadArtifactWithMime(t, svc, ctx, "sess-ok", fmt.Sprintf("f.%s", tc.name), tc.mime)
			resp, err := svc.PreviewArtifact(ctx, &v1.PreviewArtifactRequest{Id: id})
			if err != nil {
				t.Fatalf("PreviewArtifact: %v", err)
			}
			if resp.GetPreviewKind() != tc.want {
				t.Fatalf("kind=%q want %q", resp.GetPreviewKind(), tc.want)
			}
			if resp.GetDataBase64() != "" {
				t.Fatalf("%s preview must not embed base64, got %d chars", tc.name, len(resp.GetDataBase64()))
			}
		})
	}
}
