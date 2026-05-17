package codeexecutor_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/agent/codeexecutor"
)

func TestLocalExecutorUnsupportedLanguage(t *testing.T) {
	exec := codeexecutor.NewLocalExecutor(codeexecutor.LocalConfig{})
	_, err := exec.Run(context.Background(), "cobol", "hello", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestDockerExecutorUnsupportedLanguage(t *testing.T) {
	exec := codeexecutor.NewDockerExecutor(codeexecutor.DefaultDockerConfig())
	_, err := exec.Run(context.Background(), "cobol", "hello", 5*time.Second)
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
}

func TestDefaultDockerConfig(t *testing.T) {
	cfg := codeexecutor.DefaultDockerConfig()
	if cfg.Image == "" {
		t.Error("default Image should not be empty")
	}
	if cfg.Network != "none" {
		t.Errorf("expected network=none, got %q", cfg.Network)
	}
	if cfg.MemoryBytes <= 0 {
		t.Error("MemoryBytes should be positive")
	}
}
