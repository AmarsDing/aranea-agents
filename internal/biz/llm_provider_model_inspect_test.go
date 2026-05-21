package biz

import "testing"

func TestNeedInspectMerge_HunyuanNeedsSecretMerge(t *testing.T) {
	u := &LlmProviderModelUsecase{}
	in := InspectMerge{
		ProviderType: "hunyuan",
		APIBaseURL:   "https://api.hunyuan.cloud.tencent.com/v1",
	}
	if !u.needInspectMerge(in) {
		t.Fatal("expected merge when hunyuan secrets missing")
	}
}

func TestInspectCredentialsComplete(t *testing.T) {
	if !inspectCredentialsComplete(InspectMerge{ProviderType: "openai", APIKey: "sk-x"}) {
		t.Fatal("api_key should satisfy openai")
	}
	if !inspectCredentialsComplete(InspectMerge{ProviderType: "hunyuan", SecretID: "id", SecretKey: "key"}) {
		t.Fatal("hunyuan secret pair should satisfy")
	}
	if !inspectCredentialsComplete(InspectMerge{ProviderType: "ollama", APIBaseURL: "http://localhost:11434"}) {
		t.Fatal("ollama needs no api key")
	}
}
