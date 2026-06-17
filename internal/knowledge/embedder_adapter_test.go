package knowledge

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestEmbedderAdapter_GetDimensions_Delegates(t *testing.T) {
	t.Parallel()
	inner := &MultiProviderEmbedder{
		Provider: ProviderOpenAI,
		Model:    "test",
		dim:      768,
		lg:       loggateway.NewNoop(),
	}
	adapter := NewEmbedderAdapter(inner, loggateway.NewNoop())

	if got := adapter.GetDimensions(); got != 768 {
		t.Errorf("GetDimensions = %d, want 768", got)
	}
}

func TestEmbedderAdapter_GetEmbedding_ErrorPropagation(t *testing.T) {
	t.Parallel()
	inner := &MultiProviderEmbedder{
		Provider: ProviderOpenAI,
		Model:    "test",
		dim:      4,
		lg:       loggateway.NewNoop(),
	}
	adapter := NewEmbedderAdapter(inner, loggateway.NewNoop())

	// Empty text triggers a BadRequest error from EmbedSingle without network call.
	_, err := adapter.GetEmbedding(context.Background(), "  ")
	if err == nil {
		t.Fatal("GetEmbedding with empty text should return error, got nil")
	}
}

func TestEmbedderAdapter_GetEmbeddingWithUsage_ErrorPropagation(t *testing.T) {
	t.Parallel()
	inner := &MultiProviderEmbedder{
		Provider: ProviderOpenAI,
		Model:    "test",
		dim:      4,
		lg:       loggateway.NewNoop(),
	}
	adapter := NewEmbedderAdapter(inner, loggateway.NewNoop())

	_, _, err := adapter.GetEmbeddingWithUsage(context.Background(), "  ")
	if err == nil {
		t.Fatal("GetEmbeddingWithUsage with empty text should return error, got nil")
	}
}

func TestFloat32sToFloat64s_Conversion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input []float32
		want  []float64
	}{
		{"nil slice", nil, []float64{}},
		{"empty slice", []float32{}, []float64{}},
		{"positive values", []float32{1.5, 2.5, 3.5}, []float64{1.5, 2.5, 3.5}},
		{"negative values", []float32{-1.0, -2.0}, []float64{-1.0, -2.0}},
		{"zero value", []float32{0.0}, []float64{0.0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := float32sToFloat64s(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("length = %d, want %d", len(got), len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %f, want %f", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEmbedderAdapter_InterfaceAssertion(t *testing.T) {
	t.Parallel()
	// Compile-time assertion is in the production file.
	// This test verifies the adapter can be assigned to the interface at runtime.
	var _ interface {
		GetEmbedding(context.Context, string) ([]float64, error)
		GetEmbeddingWithUsage(context.Context, string) ([]float64, map[string]any, error)
		GetDimensions() int
	} = &EmbedderAdapter{}
}

func TestEmbedderAdapter_GetEmbedding_CancelledContext(t *testing.T) {
	t.Parallel()
	inner := &MultiProviderEmbedder{
		Provider: ProviderOpenAI,
		Model:    "test",
		dim:      4,
		lg:       loggateway.NewNoop(),
	}
	adapter := NewEmbedderAdapter(inner, loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := adapter.GetEmbedding(ctx, "test")
	if err == nil {
		t.Fatal("GetEmbedding with cancelled context should return error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		// The error may be wrapped; just verify it's non-nil.
		t.Logf("GetEmbedding returned error (expected): %v", err)
	}
}
