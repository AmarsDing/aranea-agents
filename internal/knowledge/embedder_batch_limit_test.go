package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestEmbedOpenAI_ModelBatchLimitPreservesOrderAndUsage(t *testing.T) {
	for _, tc := range []struct {
		model string
		count int
		limit int
		want  []int
	}{
		{"text-embedding-v4", 25, 10, []int{10, 10, 5}},
		{"text-embedding-3-small", 65, 32, []int{32, 32, 1}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var mu sync.Mutex
			var batches []int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Input []string `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				mu.Lock()
				batches = append(batches, len(request.Input))
				mu.Unlock()
				if len(request.Input) > tc.limit {
					http.Error(w, "batch size must not exceed 10", http.StatusBadRequest)
					return
				}
				var data []map[string]any
				// Providers may return each batch in a different order.
				for i := len(request.Input) - 1; i >= 0; i-- {
					value, err := strconv.Atoi(request.Input[i])
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					data = append(data, map[string]any{"index": i, "embedding": []float32{float32(value)}})
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": data, "usage": map[string]int{"prompt_tokens": len(request.Input) * 7},
				})
			}))
			defer srv.Close()
			recorder := &captureUsageRecorder{}
			embedder := NewMultiProviderEmbedder(ProviderOpenAI, srv.URL, "test-key", tc.model, 1, loggateway.NewNoop())
			embedder.SetUsageRecorder(recorder)
			texts := make([]string, tc.count)
			for i := range texts {
				texts[i] = fmt.Sprint(i + 1)
			}
			vectors, err := embedder.Embed(context.Background(), texts)
			if err != nil {
				t.Fatalf("embed %d texts: %v", tc.count, err)
			}
			mu.Lock()
			received := append([]int(nil), batches...)
			mu.Unlock()
			// 2026-11-01 chunk 并发化后到达顺序不确定（输出向量顺序仍由
			// 下标 copy 保证，见下方断言）；此处按多重集校验分块大小。
			sortedRecv := append([]int(nil), received...)
			sortedWant := append([]int(nil), tc.want...)
			sort.Ints(sortedRecv)
			sort.Ints(sortedWant)
			if !reflect.DeepEqual(sortedRecv, sortedWant) {
				t.Fatalf("batch sizes = %v, want multiset %v", received, tc.want)
			}
			if len(vectors) != tc.count {
				t.Fatalf("vectors = %d, want %d", len(vectors), tc.count)
			}
			for i, vector := range vectors {
				if len(vector) != 1 || vector[0] != float32(i+1) {
					t.Fatalf("vector %d = %v, input/result order lost", i, vector)
				}
			}
			if len(recorder.calls) != 1 || recorder.last().Tokens != tc.count*7 {
				t.Fatalf("usage = %+v, want one record with %d tokens", recorder.calls, tc.count*7)
			}
		})
	}
}
