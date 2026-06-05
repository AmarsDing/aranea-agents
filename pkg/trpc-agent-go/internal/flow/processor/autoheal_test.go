//
// Tencent is pleased to support the open source community by making trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package processor

import (
	"testing"
	"time"
)

func TestDefaultAutoHealConfig(t *testing.T) {
	cfg := DefaultAutoHealConfig()
	if !cfg.Enabled {
		t.Error("DefaultAutoHealConfig().Enabled = false, want true")
	}
	if cfg.MaxAttempts != 3 {
		t.Errorf("DefaultAutoHealConfig().MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialBackoff != 2*time.Second {
		t.Errorf("DefaultAutoHealConfig().InitialBackoff = %v, want 2s", cfg.InitialBackoff)
	}
	if cfg.BackoffFactor != 2.0 {
		t.Errorf("DefaultAutoHealConfig().BackoffFactor = %f, want 2.0", cfg.BackoffFactor)
	}
	if cfg.MaxBackoff != 30*time.Second {
		t.Errorf("DefaultAutoHealConfig().MaxBackoff = %v, want 30s", cfg.MaxBackoff)
	}
}

func TestComputeBackoff_Exponential(t *testing.T) {
	cfg := DefaultAutoHealConfig()
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 2 * time.Second},
		{attempt: 2, want: 4 * time.Second},
		{attempt: 3, want: 8 * time.Second},
	}
	for _, tt := range tests {
		got := cfg.ComputeBackoff(tt.attempt)
		if got != tt.want {
			t.Errorf("ComputeBackoff(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestComputeBackoff_MaxBackoff(t *testing.T) {
	cfg := DefaultAutoHealConfig()
	got := cfg.ComputeBackoff(10)
	if got != cfg.MaxBackoff {
		t.Errorf("ComputeBackoff(10) = %v, want %v (MaxBackoff)", got, cfg.MaxBackoff)
	}
}

func TestComputeBackoff_ZeroOrNegative(t *testing.T) {
	cfg := DefaultAutoHealConfig()
	for _, attempt := range []int{0, -1} {
		got := cfg.ComputeBackoff(attempt)
		if got != cfg.InitialBackoff {
			t.Errorf("ComputeBackoff(%d) = %v, want %v (InitialBackoff)", attempt, got, cfg.InitialBackoff)
		}
	}
}

func TestHealCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewHealCircuitBreaker(3, 10*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Error("IsOpen() = true after 2 failures, want false")
	}
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("IsOpen() = false after 3 consecutive failures, want true")
	}
}

func TestHealCircuitBreaker_ResetsAfterDuration(t *testing.T) {
	cb := NewHealCircuitBreaker(3, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Error("IsOpen() = false after 3 failures, want true")
	}
	time.Sleep(150 * time.Millisecond)
	if cb.IsOpen() {
		t.Error("IsOpen() = true after resetDuration, want false")
	}
}

func TestHealCircuitBreaker_SuccessResets(t *testing.T) {
	cb := NewHealCircuitBreaker(3, 10*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Error("IsOpen() = true after 2 failures, want false")
	}
	cb.RecordSuccess()
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Error("IsOpen() = true after success reset + 1 failure, want false")
	}
}
