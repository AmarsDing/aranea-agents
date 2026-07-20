package main

import (
	"errors"
	"strings"
	"testing"
)

// TestRedactConfigError_RedactsYAMLValueEcho guards against YAML unmarshal
// errors echoing secret values (yaml.v3 includes the offending scalar in
// type-mismatch errors, e.g. cannot unmarshal !!str `sk-...` into int).
func TestRedactConfigError_RedactsYAMLValueEcho(t *testing.T) {
	raw := errors.New("yaml: unmarshal errors:\n  line 12: cannot unmarshal !!str `sk-1234567890abcdef` into int")
	err := redactConfigError("scan config", raw)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if strings.Contains(err.Error(), "sk-1234567890abcdef") {
		t.Fatalf("secret leaked into error: %q", err.Error())
	}
	if !strings.HasPrefix(err.Error(), "scan config: ") {
		t.Fatalf("expected op prefix, got %q", err.Error())
	}
}

// TestRedactConfigError_PassthroughNonSecret keeps ordinary parse errors
// intact (only the op prefix is added).
func TestRedactConfigError_PassthroughNonSecret(t *testing.T) {
	raw := errors.New("yaml: line 3: did not find expected key")
	err := redactConfigError("load config", raw)
	if !strings.Contains(err.Error(), "did not find expected key") {
		t.Fatalf("expected original message preserved, got %q", err.Error())
	}
}

// TestRedactConfigError_RedactsDSN covers startup errors that embed a
// database DSN with password (wire/DI construction failures).
func TestRedactConfigError_RedactsDSN(t *testing.T) {
	raw := errors.New(`failed to open database: dial postgres://admin:s3cr3tpassw0rd@db.internal:5432/aranea`)
	err := redactConfigError("wire app", raw)
	if strings.Contains(err.Error(), "s3cr3tpassw0rd") {
		t.Fatalf("DSN password leaked into error: %q", err.Error())
	}
}

func TestRedactConfigError_Nil(t *testing.T) {
	if got := redactConfigError("load config", nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
}
