package provider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

func TestClassifyRetry_429(t *testing.T) {
	decision := ClassifyRetry(&http.Response{StatusCode: 429, Header: make(http.Header)}, nil)
	if decision.Type != RetryWithBackoff {
		t.Errorf("expected RetryWithBackoff for 429, got %v", decision.Type)
	}
	if !decision.IsRateLimited {
		t.Error("expected IsRateLimited=true for 429")
	}
}

func TestClassifyRetry_5xx(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504} {
		decision := ClassifyRetry(&http.Response{StatusCode: code, Header: make(http.Header)}, nil)
		if decision.Type != RetryWithBackoff {
			t.Errorf("expected RetryWithBackoff for %d, got %v", code, decision.Type)
		}
		if decision.IsRateLimited {
			t.Errorf("expected IsRateLimited=false for %d", code)
		}
	}
}

func TestClassifyRetry_ContextOverflow(t *testing.T) {
	err := errors.New("context length exceeded")
	decision := ClassifyRetry(nil, err)
	if decision.Type != RetryFatal {
		t.Errorf("expected RetryFatal for context overflow, got %v", decision.Type)
	}
}

func TestClassifyRetry_ContextCancelled(t *testing.T) {
	decision := ClassifyRetry(nil, context.Canceled)
	if decision.Type != RetryFatal {
		t.Errorf("expected RetryFatal for context.Canceled, got %v", decision.Type)
	}
}

func TestClassifyRetry_TransientNetError(t *testing.T) {
	var netErr net.Error = &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}
	decision := ClassifyRetry(nil, netErr)
	if decision.Type != RetryWithBackoff {
		t.Errorf("expected RetryWithBackoff for transient net error, got %v", decision.Type)
	}
}

func TestClassifyRetry_Unauthorized(t *testing.T) {
	decision := ClassifyRetry(&http.Response{StatusCode: 401, Header: make(http.Header)}, nil)
	if decision.Type != RetryWithClientRebuild {
		t.Errorf("expected RetryWithClientRebuild for 401, got %v", decision.Type)
	}
}

func TestClassifyRetry_ContentFilter(t *testing.T) {
	err := errors.New("content_filter: output blocked by safety system")
	decision := ClassifyRetry(nil, err)
	if decision.Type != EmitToSession {
		t.Errorf("expected EmitToSession for content filter, got %v", decision.Type)
	}
}

func TestClassifyRetry_ImageError(t *testing.T) {
	err := errors.New("unsupported image format: webp not supported by model")
	decision := ClassifyRetry(nil, err)
	if decision.Type != RetryWithImageStrip {
		t.Errorf("expected RetryWithImageStrip for image error, got %v", decision.Type)
	}
}

func TestClassifyRetry_Other4xxFatal(t *testing.T) {
	decision := ClassifyRetry(&http.Response{StatusCode: 400, Header: make(http.Header)}, nil)
	if decision.Type != RetryFatal {
		t.Errorf("expected RetryFatal for 400, got %v", decision.Type)
	}
}

func TestClassifyRetry_UnknownErrorDefaultsToBackoff(t *testing.T) {
	decision := ClassifyRetry(nil, errors.New("some unknown transient failure"))
	if decision.Type != RetryWithBackoff {
		t.Errorf("expected RetryWithBackoff for unknown error, got %v", decision.Type)
	}
}
