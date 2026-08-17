package tools

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// IsRetryableTool reports whether a tool may be retried after a transient
// failure. File-write and hostexec-family tools are not retryable: repeating
// them can double-apply side effects. ConcurrentSafe reads (including
// network fetch/search once marked concurrent) may retry.
func IsRetryableTool(name string) bool {
	if name == "" {
		return false
	}
	if IsolationStrategyForTool(name) == IsolationStrategyWorktree {
		return false
	}
	switch ExclusiveMutexKey(name) {
	case "hostexec", "workspace_exec":
		return false
	}
	return ClassifyTool(name) == SafetyConcurrentSafe
}

// SelectiveRetryOn is the product RetryOn policy: ConcurrentSafe tools retry
// on framework DefaultRetryOn (EOF / unexpected EOF / net.Error timeout or
// temporary) and on product-layer transient failures that DefaultRetryOn
// misses — wrapped `%v` network errors (no unwrap) and result-shaped HTTP
// 429/5xx or timeout strings (web_fetch returns `(result, nil)`).
// Exclusive mutating tools never retry even when the error looks transient.
func SelectiveRetryOn(ctx context.Context, info *trpctool.RetryInfo) (bool, error) {
	if info == nil || !IsRetryableTool(info.ToolName) {
		return false, nil
	}
	if ctx != nil && ctx.Err() != nil {
		return false, nil
	}
	if info.Error != nil {
		if errors.Is(info.Error, context.Canceled) || errors.Is(info.Error, context.DeadlineExceeded) {
			return false, nil
		}
	}
	retry, err := trpctool.DefaultRetryOn(ctx, info)
	if err != nil {
		return false, err
	}
	if retry {
		return true, nil
	}
	return isTransientToolFailure(info.Error, info.Result), nil
}

// FlagTransientResult marks a ConcurrentSafe tool result as a result-level
// failure when it looks transient (HTTP 429/5xx, timeout text). The wrapper
// implements RetryResultError so the framework retry runner does not treat
// `(result, nil)` as success. JSON serialization unwraps to the original
// payload. Exclusive tools and non-transient results are returned unchanged.
func FlagTransientResult(name string, result any) any {
	if result == nil || !IsRetryableTool(name) {
		return result
	}
	if _, already := result.(retryFlaggedResult); already {
		return result
	}
	if !resultLooksTransient(result) {
		return result
	}
	return retryFlaggedResult{inner: result}
}

// retryFlaggedResult tells the framework retry runner that a nil-error tool
// result is still a failure worth retrying, without changing the JSON the
// model eventually sees.
type retryFlaggedResult struct {
	inner any
}

func (r retryFlaggedResult) RetryResultError() bool { return true }

func (r retryFlaggedResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.inner)
}

func isTransientToolFailure(err error, result any) bool {
	if err != nil && isTransientFailureText(err.Error()) {
		return true
	}
	return resultLooksTransient(result)
}

func resultLooksTransient(result any) bool {
	if result == nil {
		return false
	}
	if flagged, ok := result.(retryFlaggedResult); ok {
		return resultLooksTransient(flagged.inner)
	}
	if s, ok := result.(string); ok {
		return isTransientFailureText(s)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return false
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return false
	}
	return jsonLooksTransient(decoded)
}

func jsonLooksTransient(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		if status, ok := jsonStatusCode(t); ok && isTransientHTTPStatus(status) {
			return true
		}
		if errText, ok := jsonErrorText(t); ok && isTransientFailureText(errText) {
			return true
		}
		for _, key := range []string{"results", "Results", "items", "Items"} {
			arr, ok := t[key].([]any)
			if !ok {
				continue
			}
			for _, item := range arr {
				if jsonLooksTransient(item) {
					return true
				}
			}
		}
	case []any:
		for _, item := range t {
			if jsonLooksTransient(item) {
				return true
			}
		}
	}
	return false
}

func jsonStatusCode(m map[string]any) (int, bool) {
	for _, key := range []string{"status_code", "statusCode", "StatusCode", "http_status", "httpStatus"} {
		if v, ok := m[key]; ok {
			return asInt(v)
		}
	}
	return 0, false
}

func jsonErrorText(m map[string]any) (string, bool) {
	for _, key := range []string{"error", "err", "Error"} {
		s, ok := m[key].(string)
		if ok && s != "" {
			return s, true
		}
	}
	return "", false
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func isTransientHTTPStatus(code int) bool {
	switch code {
	case 408, 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

var httpStatusInText = regexp.MustCompile(`(?i)(?:http status|status(?:\s*code)?)\s*[:=]?\s*(\d{3})`)

func isTransientFailureText(s string) bool {
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "context canceled") && !strings.Contains(lower, "timeout") {
		return false
	}
	// Bare tool-budget deadline (no client timeout) is not retryable.
	if strings.Contains(lower, "context deadline exceeded") && !strings.Contains(lower, "timeout") {
		return false
	}
	if code := httpStatusFromText(lower); isTransientHTTPStatus(code) {
		return true
	}
	for _, phrase := range transientErrorPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func httpStatusFromText(lower string) int {
	m := httpStatusInText.FindStringSubmatch(lower)
	if len(m) < 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

var transientErrorPhrases = []string{
	"timeout",
	"timed out",
	"i/o timeout",
	"connection reset",
	"connection refused",
	"broken pipe",
	"unexpected eof",
	"temporary failure",
	"too many requests",
	"service unavailable",
	"bad gateway",
	"gateway timeout",
	"econnreset",
	"econnrefused",
	"eof",
}
