package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"

	"aranea-agents/internal/cli/clierr"
	"google.golang.org/protobuf/proto"
)

// kratosError mirrors the Kratos HTTP error response body.
type kratosError struct {
	Code    int    `json:"code"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
	// Metadata may be map[string]any in some error responses.
	Metadata map[string]any `json:"metadata"`
}

// decode reads and parses an HTTP response. If status >=300, it decodes the error body.
// If the status is >=300, it decodes the error body into a CLIError.
func decode(resp *http.Response, out proto.Message) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return wrapNetErr(err)
	}

	if resp.StatusCode < 300 {
		if out == nil || len(body) == 0 {
			return nil
		}
		if err := unmarshalOpts.Unmarshal(body, out); err != nil {
			// Fallback: maybe it's empty or non-proto JSON.
			return nil
		}
		return nil
	}

	// Error response.
	var ke kratosError
	_ = json.Unmarshal(body, &ke)

	code := ke.Reason
	if code == "" {
		code = codeFromStatus(resp.StatusCode)
	}
	msg := ke.Message
	if msg == "" {
		msg = string(body)
	}

	return &clierr.CLIError{
		Code:       code,
		HTTPStatus: resp.StatusCode,
		Message:    msg,
		Metadata:   ke.Metadata,
		Hint:       hintFromCode(code, resp.StatusCode),
	}
}

// wrapNetErr converts a network error into a *cli.CLIError with Code=NETWORK_ERROR.
func wrapNetErr(err error) error {
	if err == nil {
		return nil
	}
	if isNetErr(err) {
		return &clierr.CLIError{
			Code:    "NETWORK_ERROR",
			Message: fmt.Sprintf("network error: %v", err),
			Cause:   err,
		}
	}
	return &clierr.CLIError{
		Code:    "NETWORK_ERROR",
		Message: err.Error(),
		Cause:   err,
	}
}

func isNetErr(err error) bool {
	var netErr net.Error
	if ok := errorAs(err, &netErr); ok {
		return true
	}
	return false
}

func errorAs(err error, target any) bool {
	type unwrapper interface{ Unwrap() error }
	for err != nil {
		if netErr, ok := target.(*net.Error); ok {
			if ne, ok2 := err.(net.Error); ok2 {
				*netErr = ne
				return true
			}
		}
		u, ok := err.(unwrapper)
		if !ok {
			break
		}
		err = u.Unwrap()
	}
	return false
}

// decodeErrorBody parses a raw error body into a *cli.CLIError.
func decodeErrorBody(body []byte, status int) error {
	var ke kratosError
	_ = json.Unmarshal(body, &ke)
	code := ke.Reason
	if code == "" {
		code = codeFromStatus(status)
	}
	msg := ke.Message
	if msg == "" {
		msg = string(body)
	}
	return &clierr.CLIError{
		Code:       code,
		HTTPStatus: status,
		Message:    msg,
		Metadata:   ke.Metadata,
		Hint:       hintFromCode(code, status),
	}
}
func codeFromStatus(status int) string {
	switch status {
	case 400:
		return "BAD_REQUEST"
	case 401:
		return "UNAUTHENTICATED"
	case 403:
		return "FORBIDDEN"
	case 404:
		return "NOT_FOUND"
	case 409:
		return "CONFLICT"
	case 422:
		return "UNPROCESSABLE"
	case 429:
		return "RATE_LIMITED"
	case 500:
		return "INTERNAL"
	case 502, 503, 504:
		return "UNAVAILABLE"
	default:
		return "UNKNOWN"
	}
}

// hintFromCode returns a human-friendly hint based on error code or HTTP status.
func hintFromCode(code string, status int) string {
	switch code {
	case "UNAUTHENTICATED":
		return "run `aranea login` to authenticate"
	case "FORBIDDEN":
		return "your account lacks permission for this action; contact your administrator"
	case "NOT_FOUND":
		return "the requested resource does not exist"
	}
	switch status {
	case 401, 403:
		return "run `aranea login` to re-authenticate"
	case 500, 502, 503:
		return "the backend service may be unavailable; retry later"
	}
	return ""
}
