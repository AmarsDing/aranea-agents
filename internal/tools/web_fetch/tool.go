// Package web_fetch implements the platform catalog web_fetch tool for OpenAI-compatible ADK runs.
package web_fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/tools/argmap"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const maxBody = 256 * 1024

type fetchArgs struct {
	URL          string `json:"url"`
	ExtractMode  string `json:"extract_mode,omitempty"`
}

const desc = `Fetch a public HTTP or HTTPS URL and return status, content-type, and a truncated plain-text body preview. Use when the user gave a specific URL or asked to read a web page. Argument extract_mode is optional (ignored for now; output is always plain text).`

// New builds the ADK function tool named web_fetch.
func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "web_fetch",
		Description: desc,
	}, func(tc tool.Context, in fetchArgs) (map[string]any, error) {
		return Run(tc, map[string]any{
			"url":           in.URL,
			"extract_mode":  in.ExtractMode,
		})
	})
}

// Run fetches one URL (GET) with timeout and size limits.
func Run(ctx context.Context, args map[string]any) (map[string]any, error) {
	raw := strings.TrimSpace(argmap.String(args, "url"))
	if raw == "" {
		return nil, fmt.Errorf("url is required")
	}
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("only http and https URLs are supported")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Aranea-Agent/1.0 (web_fetch)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	const maxRunes = 8000
	r := []rune(text)
	if len(r) > maxRunes {
		text = string(r[:maxRunes]) + "..."
	}
	return map[string]any{
		"url":          raw,
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"text":        text,
	}, nil
}
