package plugintrpc

import "testing"

func TestResolveModelAPI_CodeTask(t *testing.T) {
	cfg := ModelRouterConfig{
		DefaultModel: "gpt-4o-mini",
		CodeModel:    "gpt-4o",
	}
	got := ResolveModelAPI("please fix ```go\nfunc main(){}", cfg)
	if got != "gpt-4o" {
		t.Fatalf("got %q want gpt-4o", got)
	}
}

func TestResolveModelAPI_LongContext(t *testing.T) {
	cfg := ModelRouterConfig{
		DefaultModel:     "small",
		LongContextModel: "large",
	}
	prompt := stringsRepeat("x", 12001)
	got := ResolveModelAPI(prompt, cfg)
	if got != "large" {
		t.Fatalf("got %q want large", got)
	}
}

func stringsRepeat(s string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = s[0]
	}
	return string(b)
}
