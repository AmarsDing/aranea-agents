package health

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestDefaultInterval(t *testing.T) {
	if DefaultInterval() <= 0 {
		t.Fatal("expected positive default interval")
	}
}

func TestNewRunner(t *testing.T) {
	if NewRunner(Deps{}, loggateway.NewNoop()) == nil {
		t.Fatal("expected runner")
	}
}
