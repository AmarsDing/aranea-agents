package runtime_test

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/channel/runtime"
)

func TestCredentialsRevisionChangesOnUpdate(t *testing.T) {
	a := []biz.ChannelCredential{{CredentialKey: "app_secret", UpdatedAt: "t1", SecretRef: "ref1"}}
	b := []biz.ChannelCredential{{CredentialKey: "app_secret", UpdatedAt: "t2", SecretRef: "ref1"}}
	if runtime.CredentialsRevision(a) == runtime.CredentialsRevision(b) {
		t.Fatal("expected different revision after UpdatedAt change")
	}
}
