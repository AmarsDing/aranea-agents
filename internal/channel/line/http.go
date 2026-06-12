package line

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

func doPost(ctx context.Context, client *http.Client, token, url string, body []byte) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, apierror.Internal("LINE_PROTOCOL", fmt.Sprintf("line: status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw))))
	}
	return raw, nil
}

func marshalMessages(to string, messages []map[string]any) ([]byte, error) {
	payload := map[string]any{
		"to":       to,
		"messages": messages,
	}
	return json.Marshal(payload)
}

func textMessage(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}
