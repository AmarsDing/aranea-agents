package biz

import "testing"

func TestResolvePluginVersion(t *testing.T) {
	if got := ResolvePluginVersion(PluginVersionPolicy{Pinned: "1.2.3"}, "9.9.9"); got != "1.2.3" {
		t.Fatalf("pinned: got %q", got)
	}
	if got := ResolvePluginVersion(PluginVersionPolicy{}, "2.0.0"); got != "2.0.0" {
		t.Fatalf("latest: got %q", got)
	}
}

func TestNormalizePluginSandboxMode(t *testing.T) {
	if got := NormalizePluginSandboxMode("", "high"); got != PluginSandboxProcess {
		t.Fatalf("high risk default: got %q", got)
	}
	if got := NormalizePluginSandboxMode("container", "low"); got != PluginSandboxContainer {
		t.Fatalf("explicit container: got %q", got)
	}
	if got := NormalizePluginSandboxMode("invalid", "low"); got != PluginSandboxNone {
		t.Fatalf("low risk default: got %q", got)
	}
}
