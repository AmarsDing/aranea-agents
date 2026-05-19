package plugintrpc

import "testing"

func TestPluginMatchesScope(t *testing.T) {
	if !PluginMatchesScope("", "a1") {
		t.Fatal("empty scope should match")
	}
	if !PluginMatchesScope("global", "a1") {
		t.Fatal("global should match")
	}
	if !PluginMatchesScope("a1", "a1") {
		t.Fatal("exact agent should match")
	}
	if PluginMatchesScope("a2", "a1") {
		t.Fatal("other agent should not match")
	}
	if PluginMatchesScope("a1", "") {
		t.Fatal("agent-scoped plugin should not match empty agent id")
	}
}
