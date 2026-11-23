package fleet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 离线包照官方格式拼：文件放对位置、校验通过、清单里的地址换成包内相对路径；校验不符要报错。
func TestBuildTraeBundle(t *testing.T) {
	files := map[string]string{
		"/v2/releases/1.0.0/traex-windows-x86_64.exe.gz":                "main-binary",
		"/v2/releases/1.0.0/traex-code-mode-host-windows-x86_64.exe.gz": "sidecar",
		"/v2/deps/ripgrep/14/rg-windows-x86_64.zip":                     "rg-zip",
	}
	sum := func(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
	sums := map[string]string{}
	for p, b := range files {
		sums[p] = sum(b) // 发布清单里的校验值，之后文件被换也不变
	}
	var srv *httptest.Server
	rec := func(path, comp string) string {
		return fmt.Sprintf(`{"filename":%q,"url":%q,"sha256":%q,"size":%d,"compression":%q}`,
			filepath.Base(path), srv.URL+path, sums[path], len(files[path]), comp)
	}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/public/stable/latest.json":
			main := strings.TrimSuffix(rec("/v2/releases/1.0.0/traex-windows-x86_64.exe.gz", "gzip"), "}") +
				`,"platform":"windows-x86_64","sidecars":[` + rec("/v2/releases/1.0.0/traex-code-mode-host-windows-x86_64.exe.gz", "gzip") + `]}`
			_, _ = fmt.Fprintf(w, `{"version":"1.0.0","source_manifest_sha256":"abc","artifacts":[{"platform":"macos-x86_64"},%s]}`, main)
		case "/v2/deps/ripgrep/latest.json":
			rg := strings.TrimSuffix(rec("/v2/deps/ripgrep/14/rg-windows-x86_64.zip", "zip"), "}") +
				`,"platform":"windows-x86_64","archive_path":"rg-14/rg.exe"}`
			_, _ = fmt.Fprintf(w, `{"artifacts":[%s]}`, rg)
		default:
			body, ok := files[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			http.ServeContent(w, r, "", time.Time{}, strings.NewReader(body)) // 支持 Range，测续传
		}
	}))
	defer srv.Close()

	bundle := t.TempDir()
	// 段长 4 字节：多段并行；上次第 0 段下了 2 个字节，这次应从第 3 个字节续上
	defer func(n int64) { segSize = n }(segSize)
	segSize = 4
	mainSum := sums["/v2/releases/1.0.0/traex-windows-x86_64.exe.gz"]
	_ = os.MkdirAll(filepath.Join(bundle, "artifacts"), 0o755)
	_ = os.WriteFile(filepath.Join(bundle, "artifacts", "traex-windows-x86_64.exe.gz."+mainSum[:12]+".0.part"), []byte("ma"), 0o644)
	if err := buildTraeBundle(context.Background(), srv.URL, bundle, "windows-x86_64", "script", io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"artifacts/traex-windows-x86_64.exe.gz", "artifacts/traex-code-mode-host-windows-x86_64.exe.gz",
		"dependencies/ripgrep/rg-windows-x86_64.zip", "dependencies/ripgrep/manifest.json", "offline-manifest.json", "install_v2.ps1"} {
		if !fileExists(filepath.Join(bundle, filepath.FromSlash(p))) {
			t.Fatalf("missing %s", p)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(bundle, "artifacts", "traex-windows-x86_64.exe.gz")); string(got) != "main-binary" {
		t.Fatalf("resumed file: %q", got)
	}
	var man struct {
		Version        string `json:"version"`
		OfflineChannel string `json:"offline_channel"`
		Artifacts      []struct {
			URL      string `json:"url"`
			Size     int64  `json:"size"`
			Sidecars []struct {
				URL string `json:"url"`
			} `json:"sidecars"`
		} `json:"artifacts"`
	}
	b, _ := os.ReadFile(filepath.Join(bundle, "offline-manifest.json"))
	if err := json.Unmarshal(b, &man); err != nil || man.OfflineChannel != "public" || len(man.Artifacts) != 1 ||
		man.Artifacts[0].URL != "artifacts/traex-windows-x86_64.exe.gz" || man.Artifacts[0].Size != int64(len("main-binary")) ||
		man.Artifacts[0].Sidecars[0].URL != "artifacts/traex-code-mode-host-windows-x86_64.exe.gz" {
		t.Fatalf("manifest: %+v err=%v\n%s", man, err, b)
	}

	// 服务器不再发数据：看门狗到点就放弃，报「没有进展」而不是一直挂着
	stallTimeout = 200 * time.Millisecond
	defer func() { stallTimeout = 3 * time.Minute }()
	hang := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "x")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer hang.Close()
	if err := downloadVerified(context.Background(), hang.URL, filepath.Join(t.TempDir(), "f"), 100, sums["/v2/deps/ripgrep/14/rg-windows-x86_64.zip"], io.Discard); err == nil || !strings.Contains(err.Error(), "没有进展") {
		t.Fatalf("stall: %v", err)
	}

	files["/v2/releases/1.0.0/traex-windows-x86_64.exe.gz"] = "tampered!!!" // 同样大小、内容被换
	if err := buildTraeBundle(context.Background(), srv.URL, t.TempDir(), "windows-x86_64", "script", io.Discard); err == nil {
		t.Fatal("a checksum mismatch must fail")
	}
}
