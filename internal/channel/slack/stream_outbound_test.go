package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamSenderUpdateThrottlesEdits(t *testing.T) {
	var posts, updates int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "chat.postMessage"):
			posts++
			_, _ = w.Write([]byte(`{"ok":true,"ts":"123.456"}`))
		case strings.Contains(r.URL.Path, "chat.update"):
			updates++
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: http.DefaultTransport, host: srv.URL}
	client := &http.Client{Transport: transport}
	s := &StreamSender{BotToken: "xoxb-test", HTTP: client, EditInterval: time.Millisecond}

	if err := s.Update(context.Background(), "C123", "hello", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := s.Update(context.Background(), "C123", "hello world", false); err != nil {
		t.Fatal(err)
	}
	if posts != 1 || updates != 1 {
		t.Fatalf("posts=%d updates=%d", posts, updates)
	}
	if err := s.Update(context.Background(), "C123", "hello world!", true); err != nil {
		t.Fatal(err)
	}
	if updates != 2 {
		t.Fatalf("expected final flush update, updates=%d", updates)
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
