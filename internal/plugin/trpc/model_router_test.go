package plugintrpc

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

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

// I-1 回归：adapt() 必须编译 model_router 规则的 regex，否则实际路由路径
// （PluginModelSelector → Runtime 条目 config）上 regex 规则静默失效。
func TestAdapt_ModelRouterCompilesRegexRules(t *testing.T) {
	p := biz.Plugin{
		Key:        "model_router",
		Enabled:    true,
		ConfigJSON: `{"rules":[{"model":"deepseek-v3","regex":"(?i)sql|数据库","priority":10}]}`,
	}
	ap := adapt(p, nil, nil, nil, loggateway.NewNoop())
	if ap == nil || ap.modelRouter == nil {
		t.Fatal("adapt returned nil modelRouter config")
	}
	got := ResolveModelAPI("请优化这条 SQL 查询", *ap.modelRouter)
	if got != "deepseek-v3" {
		t.Fatalf("regex rule not compiled by adapt: got %q, want deepseek-v3", got)
	}
}
