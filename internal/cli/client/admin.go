package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	adminv1 "aranea-agents/api/kratos/admin/v1"
)
// Login calls POST /v1/admins/login and extracts the token from the Set-Cookie header.
//
// Code archaeology A1: The backend Login handler calls auth.SetCookie which sets a
// JWT token in the HTTP Set-Cookie response header (cookie name from KRATOS_AUTH_COOKIE
// env var, default "access_token"). The response body returns the Admin object (id/name/email).
// Token is NOT in the body. CLI must extract it from Set-Cookie.
func (c *Client) Login(ctx context.Context, username, password string) (*adminv1.Admin, string, error) {
	reqBody := fmt.Sprintf(`{"username":"%s","password":"%s"}`, escapeJSON(username), escapeJSON(password))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/admins/login",
		bytes.NewReader([]byte(reqBody)))
	if err != nil {
		return nil, "", fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)

	if c.Debug {
		c.logRequest(req)
	}

	resp, err := c.Doer.Do(req)
	if err != nil {
		return nil, "", wrapNetErr(err)
	}
	defer resp.Body.Close()

	if c.Debug {
		c.logResponse(resp)
	}

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return nil, "", decodeErrorBody(body, resp.StatusCode)
	}

	// Extract token from Set-Cookie.
	token := extractTokenFromCookies(resp.Cookies())
	if token == "" {
		// Fallback: try Authorization header.
		token = extractBearerFromHeader(resp.Header.Get("Authorization"))
	}

	// Parse admin body.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, token, nil
	}
	admin := &adminv1.Admin{}
	if err := unmarshalOpts.Unmarshal(body, admin); err != nil {
		// Body parse failure is non-fatal if we have a token.
		return admin, token, nil
	}
	return admin, token, nil
}

// CurrentAdmin calls GET /v1/admins/current and returns the current user.
func (c *Client) CurrentAdmin(ctx context.Context) (*adminv1.Admin, error) {
	admin := &adminv1.Admin{}
	if err := c.Do(ctx, http.MethodGet, "/v1/admins/current", nil, admin); err != nil {
		return nil, err
	}
	return admin, nil
}

// extractTokenFromCookies looks for the JWT token in the response cookies.
// The backend cookie name is KRATOS_AUTH_COOKIE (default "access_token").
func extractTokenFromCookies(cookies []*http.Cookie) string {
	// Try common cookie names in priority order.
	names := []string{"access_token", "session", "token", "jwt"}
	byName := make(map[string]string, len(cookies))
	for _, c := range cookies {
		byName[c.Name] = c.Value
	}
	for _, name := range names {
		if v, ok := byName[name]; ok && v != "" {
			return v
		}
	}
	// Return first non-empty cookie value.
	for _, c := range cookies {
		if c.Value != "" {
			return c.Value
		}
	}
	return ""
}

// extractBearerFromHeader strips the "Bearer " prefix from an Authorization header value.
func extractBearerFromHeader(header string) string {
	const prefix = "Bearer "
	if len(header) > len(prefix) {
		return header[len(prefix):]
	}
	return ""
}

// escapeJSON escapes special JSON characters in a string value.
func escapeJSON(s string) string {
	out := make([]byte, 0, len(s)+4)
	for _, b := range []byte(s) {
		switch b {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, b)
		}
	}
	return string(out)
}
