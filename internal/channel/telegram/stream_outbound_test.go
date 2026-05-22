package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamSenderUpdateThrottlesEdits(t *testing.T) {
	var sends, edits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "sendMessage"):
			sends++
			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
		case strings.Contains(r.URL.Path, "editMessageText"):
			edits++
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: http.DefaultTransport, host: srv.URL}
	client := &http.Client{Transport: transport}
	s := &StreamSender{BotToken: "tok", HTTP: client, EditInterval: time.Millisecond}

	if err := s.Update(context.Background(), "100", "hello", false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := s.Update(context.Background(), "100", "hello world", false); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || edits != 1 {
		t.Fatalf("sends=%d edits=%d", sends, edits)
	}
	if err := s.Update(context.Background(), "100", "hello world!", true); err != nil {
		t.Fatal(err)
	}
	if edits != 2 {
		t.Fatalf("expected final flush edit, edits=%d", edits)
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
