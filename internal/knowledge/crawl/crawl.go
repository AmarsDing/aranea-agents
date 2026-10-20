// Package crawl provides a small URL/site crawling engine: a page fetcher
// with content-type and size limits, an RFC 9309 robots.txt subset parser,
// a sitemap parser, and a concurrent breadth-first crawl scheduler.
//
// The package is a pure library. It fetches URLs and hands payloads to a
// caller-provided sink; wiring into the ingest pipeline happens elsewhere.
package crawl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	// DefaultMaxBytes caps a fetched body at 32 MiB unless overridden.
	DefaultMaxBytes = 32 << 20
	// DefaultTimeout bounds the whole HTTP exchange unless overridden.
	DefaultTimeout = 30 * time.Second
	// DefaultUserAgent is sent when Client.UserAgent is empty.
	DefaultUserAgent = "aranea-crawler/1.0"
)

var (
	// ErrUnsupportedMIME is returned (wrapped) when the response
	// Content-Type is not on the fetch whitelist.
	ErrUnsupportedMIME = errors.New("crawl: unsupported content type")
	// ErrTooLarge is returned (wrapped) when the response body exceeds
	// Client.MaxBytes.
	ErrTooLarge = errors.New("crawl: response body too large")
)

// supportedMIME lists the Content-Types FetchPage accepts.
var supportedMIME = map[string]struct{}{
	"text/html":             {},
	"text/plain":            {},
	"application/pdf":       {},
	"text/markdown":         {},
	"application/xhtml+xml": {},
}

// Client fetches pages over HTTP with type and size limits.
type Client struct {
	// HC is the underlying HTTP client. When nil, one is built per call
	// using Timeout.
	HC *http.Client
	// UserAgent is sent as the User-Agent header (default DefaultUserAgent).
	UserAgent string
	// MaxBytes caps the response body size (default 32 MiB).
	MaxBytes int64
	// Timeout bounds the HTTP exchange (default 30s); only used to build
	// an http.Client when HC is nil.
	Timeout time.Duration
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return DefaultUserAgent
}

func (c *Client) maxBytes() int64 {
	if c.MaxBytes > 0 {
		return c.MaxBytes
	}
	return DefaultMaxBytes
}

func (c *Client) httpClient() *http.Client {
	if c.HC != nil {
		return c.HC
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &http.Client{Timeout: timeout}
}

// Payload is a fetched page.
type Payload struct {
	URL      string // requested URL
	FinalURL string // URL after redirects (empty means same as URL)
	MIME     string // normalized Content-Type, parameters stripped
	Title    string // <title> text for HTML payloads
	Body     []byte
}

// FetchPage downloads rawurl following redirects (recording the final URL).
// Responses whose Content-Type is off the whitelist fail with
// ErrUnsupportedMIME; bodies exceeding MaxBytes fail with ErrTooLarge.
// For HTML responses the <title> element text is captured in Payload.Title.
func (c *Client) FetchPage(ctx context.Context, rawurl string) (Payload, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return Payload{}, fmt.Errorf("crawl: build request for %q: %w", rawurl, err)
	}
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Payload{}, fmt.Errorf("crawl: fetch %q: %w", rawurl, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return Payload{}, fmt.Errorf("crawl: fetch %q: unexpected status %s", rawurl, resp.Status)
	}

	mime := parseMIME(resp.Header.Get("Content-Type"))
	if _, ok := supportedMIME[mime]; !ok {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return Payload{}, fmt.Errorf("crawl: fetch %q: %w: %q", rawurl, ErrUnsupportedMIME, mime)
	}

	max := c.maxBytes()
	body, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return Payload{}, fmt.Errorf("crawl: read body of %q: %w", rawurl, err)
	}
	if int64(len(body)) > max {
		return Payload{}, fmt.Errorf("crawl: fetch %q: %w: body exceeds %d bytes", rawurl, ErrTooLarge, max)
	}

	p := Payload{
		URL:      rawurl,
		FinalURL: resp.Request.URL.String(),
		MIME:     mime,
		Body:     body,
	}
	if mime == "text/html" || mime == "application/xhtml+xml" {
		p.Title = extractTitle(body)
	}
	return p, nil
}

// parseMIME strips parameters (e.g. "; charset=utf-8") and normalizes case.
func parseMIME(contentType string) string {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	return strings.ToLower(strings.TrimSpace(contentType))
}

// extractTitle returns the trimmed text of the first <title> element.
func extractTitle(body []byte) string {
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" {
			var sb strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					sb.WriteString(c.Data)
				}
			}
			title = strings.TrimSpace(sb.String())
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return title
}
