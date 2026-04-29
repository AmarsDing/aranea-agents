package backends

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"arenea/backend/internal/capability/schema"
	"arenea/backend/internal/capability/toolctx"
)

const (
	datetimeToolDescription = "Return the current local and UTC clock time as RFC3339 strings. " +
		"Use only when the user asks for the current time or the answer depends on now. " +
		"DO NOT call to ground unrelated reasoning; the Runtime Context already records when the session started."
	webFetchToolDescription = "Fetch a single public HTTP or HTTPS URL and return a truncated plain-text preview. " +
		"Argument: url (string). Use only when the user explicitly provided a URL or asked you to look something up online. " +
		"DO NOT use to query application data, internal databases or anything described in the Runtime Context. " +
		"If the same URL fails twice, stop and report the limitation instead of retrying."
)

type DateTimeTool struct{ Base }
type WebFetchTool struct{ Base }

func NewDateTimeTool() *DateTimeTool {
	return &DateTimeTool{Base: Base{Key: "datetime", Label: "当前时间", Desc: datetimeToolDescription, ToolCategory: "system", InSchema: schema.JSONSchemaOf[schema.EmptyInput](), OutSchema: schema.JSONSchemaOf[schema.DateTimeOutput]()}}
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{Base: Base{Key: "web_fetch", Label: "Web 抓取", Desc: webFetchToolDescription, ToolCategory: "web", Required: []string{"url"}, InSchema: schema.JSONSchemaOf[schema.WebFetchInput](), OutSchema: schema.JSONSchemaOf[schema.WebFetchOutput]()}}
}

func (t *DateTimeTool) Execute(_ *toolctx.ToolContext, _ map[string]any) (map[string]any, error) {
	now := time.Now()
	return map[string]any{"local": now.Format(time.RFC3339), "utc": now.UTC().Format(time.RFC3339)}, nil
}

func (t *WebFetchTool) Execute(ctx *toolctx.ToolContext, params map[string]any) (map[string]any, error) {
	rawURL := stringParam(params, "url")
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	lower := strings.ToLower(rawURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("only http and https URLs are supported")
	}
	reqCtx := context.Context(nil)
	if ctx != nil {
		reqCtx = ctx.Context
	}
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Aranea-Agent/1.0")
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	if len([]rune(text)) > 6000 {
		runes := []rune(text)
		text = string(runes[:6000]) + "..."
	}
	return map[string]any{"url": rawURL, "status_code": resp.StatusCode, "content_type": resp.Header.Get("Content-Type"), "text": text}, nil
}
