package health

import "testing"

func TestDefaultInterval(t *testing.T) {
	if DefaultInterval() <= 0 {
		t.Fatal("expected positive default interval")
	}
}

func TestNewRunner(t *testing.T) {
	if NewRunner(Deps{}) == nil {
		t.Fatal("expected runner")
	}
}
