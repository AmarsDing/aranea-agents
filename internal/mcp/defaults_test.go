package mcp_test

import (
	"testing"
	"time"

	"aranea-agents/internal/mcp"
)

func TestDefaultProbeTimeoutSec(t *testing.T) {
	if mcp.DefaultProbeTimeoutSec != 10 {
		t.Errorf("DefaultProbeTimeoutSec = %d, want 10", mcp.DefaultProbeTimeoutSec)
	}
}

func TestDefaultHealthInterval(t *testing.T) {
	if mcp.DefaultHealthInterval != 5*time.Minute {
		t.Errorf("DefaultHealthInterval = %v, want %v", mcp.DefaultHealthInterval, 5*time.Minute)
	}
}

func TestDefaultSustainedErrorAfter(t *testing.T) {
	if mcp.DefaultSustainedErrorAfter != 5*time.Minute {
		t.Errorf("DefaultSustainedErrorAfter = %v, want %v", mcp.DefaultSustainedErrorAfter, 5*time.Minute)
	}
}

func TestDefaultSessionReconnectMax(t *testing.T) {
	if mcp.DefaultSessionReconnectMax != 3 {
		t.Errorf("DefaultSessionReconnectMax = %d, want 3", mcp.DefaultSessionReconnectMax)
	}
}

func TestDefaultOAuth2TimeoutSec(t *testing.T) {
	if mcp.DefaultOAuth2TimeoutSec != 15 {
		t.Errorf("DefaultOAuth2TimeoutSec = %d, want 15", mcp.DefaultOAuth2TimeoutSec)
	}
}

func TestDefaultRuntimeTimeoutSec(t *testing.T) {
	if mcp.DefaultRuntimeTimeoutSec != 60 {
		t.Errorf("DefaultRuntimeTimeoutSec = %d, want 60", mcp.DefaultRuntimeTimeoutSec)
	}
}

func TestRecentReconnectWindow(t *testing.T) {
	if mcp.RecentReconnectWindow != 24*time.Hour {
		t.Errorf("RecentReconnectWindow = %v, want %v", mcp.RecentReconnectWindow, 24*time.Hour)
	}
}

func TestDurationConstantsPositive(t *testing.T) {
	durations := map[string]time.Duration{
		"DefaultHealthInterval":      mcp.DefaultHealthInterval,
		"DefaultSustainedErrorAfter": mcp.DefaultSustainedErrorAfter,
		"RecentReconnectWindow":      mcp.RecentReconnectWindow,
	}
	for name, d := range durations {
		if d <= 0 {
			t.Errorf("%s = %v, want positive duration", name, d)
		}
	}
}

func TestIntegerConstantsPositive(t *testing.T) {
	integers := map[string]int{
		"DefaultProbeTimeoutSec":     mcp.DefaultProbeTimeoutSec,
		"DefaultSessionReconnectMax": mcp.DefaultSessionReconnectMax,
		"DefaultOAuth2TimeoutSec":    mcp.DefaultOAuth2TimeoutSec,
		"DefaultRuntimeTimeoutSec":   mcp.DefaultRuntimeTimeoutSec,
	}
	for name, v := range integers {
		if v <= 0 {
			t.Errorf("%s = %d, want positive integer", name, v)
		}
	}
}
