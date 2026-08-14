package computeruse

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

func TestSpecialistGrounderPickCoordinate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ground" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]int{"x": 120, "y": 340})
	}))
	t.Cleanup(srv.Close)

	g := NewSpecialistGrounder(srv.URL, loggateway.NewNoop())
	pt, err := g.PickCoordinate(context.Background(), bizcu.Image{PNG: []byte("png"), Width: 800, Height: 600}, "保存")
	if err != nil {
		t.Fatalf("PickCoordinate: %v", err)
	}
	if pt.X != 120 || pt.Y != 340 {
		t.Errorf("pt = %+v", pt)
	}
}

func TestSpecialistGrounderNegativeSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]int{"x": -1, "y": -1})
	}))
	t.Cleanup(srv.Close)
	g := NewSpecialistGrounder(srv.URL, loggateway.NewNoop())
	_, err := g.PickCoordinate(context.Background(), bizcu.Image{PNG: []byte("x")}, "无")
	if err == nil {
		t.Fatal("expected grounding failed")
	}
}
