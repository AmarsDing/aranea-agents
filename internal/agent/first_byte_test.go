package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type stubFirstByteCatalog struct {
	cfg string
	err error
}

func (s stubFirstByteCatalog) GetByProviderAndModel(_ context.Context, _, _ string) (biz.ProviderModel, error) {
	if s.err != nil {
		return biz.ProviderModel{}, s.err
	}
	return biz.ProviderModel{ConfigJSON: s.cfg}, nil
}

func (s stubFirstByteCatalog) List(context.Context) ([]biz.ProviderModel, error) {
	return nil, nil
}

func TestResolveFirstByteTimeout(t *testing.T) {
	if got := ResolveFirstByteTimeout(context.Background(), nil, "p", "m"); got != DefaultFirstByteTimeout {
		t.Fatalf("nil catalog = %v, want default", got)
	}
	miss := stubFirstByteCatalog{err: apierror.NotFound("LLM_PROVIDER_MODEL", "missing")}
	if got := ResolveFirstByteTimeout(context.Background(), miss, "p", "m"); got != DefaultFirstByteTimeout {
		t.Fatalf("catalog miss = %v, want default", got)
	}
	pack := stubFirstByteCatalog{cfg: `{"first_byte_timeout_sec":75}`}
	if got := ResolveFirstByteTimeout(context.Background(), pack, "p", "m"); got != 75*time.Second {
		t.Fatalf("pack overlay = %v, want 75s", got)
	}
}
