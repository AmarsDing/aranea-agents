package lark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamSenderUpdateThrottlesEdits(t *testing.T) {
	var sends, patches int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/messages"):
			sends++
			_, _ = w.Write([]byte(`{"code":0,"data":{"message_id":"om_test"}}`))
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/messages/"):
			patches++
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: http.DefaultTransport, host: srv.URL}
	client := &http.Client{Transport: transport}
	s := &StreamSender{
		Region:       RegionFeishu,
		AppID:        "app",
		AppSecret:    "sec",
		HTTP:         client,
		EditInterval: time.Millisecond,
		tenantTok:    "tok",
		tokenUntil:   time.Now().Add(time.Hour),
	}

	if err := s.Update(context.Background(), "ou_test", "hello", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := s.Update(context.Background(), "ou_test", "hello world", false); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || patches != 1 {
		t.Fatalf("sends=%d patches=%d", sends, patches)
	}
	if err := s.Update(context.Background(), "ou_test", "hello world!", true); err != nil {
		t.Fatal(err)
	}
	if patches != 2 {
		t.Fatalf("expected final flush patch, patches=%d", patches)
	}
}

type rewriteTransport struct {
	base http.RoundTripper
	host string
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(strings.TrimPrefix(t.host, "https://"), "http://")
	if t.base == nil {
		t.base = http.DefaultTransport
	}
	return t.base.RoundTrip(req)
}
