// Package eval productizes retrieval golden-set evaluation as a pure library:
// it scores a Searcher against curated cases and renders/diffs reports.
//
// The package has zero external side effects: no RPC wiring, no database
// access, no logging. It is planned to be exposed through the service/RPC
// layer by the full U1-3 rollout; until then callers compose it directly.
//
// G-13 快照门禁：fixture_search.go 提供无 PG/embedder 依赖的确定性检索器
// （种子锚点 + 生产成链/切分/一跳扩展/合并组件），testdata 入仓语料 + 评测集 +
// 快照，gold_snapshot_test.go 在 `go test ./...` 内做 before/after 门禁：chunk
// recall 与 chunk MRR 跌幅、检索失败、集漂移、基线降级全卡；判定单一事实源在
// gate.go（CheckGoldGate / ValidateGoldBaseline / ValidateGoldExpectations，
// 与 cmd/goldeval 共用）。cmd/goldeval 为驱动命令（CI 步 + 本地 -update 刷新，
// 拒写带错/降级基线）。
package eval

import (
	"fmt"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// GoldenCase is one curated expectation for a retrieval query. Expected ids
// may be given at chunk granularity, document granularity, or both; the
// runner scores both bases independently. An empty expected list means
// "nothing to recall" and every metric returns 1.0 (vacuous success).
type GoldenCase struct {
	ID               string   `json:"id"`
	Query            string   `json:"query"`
	ExpectedChunkIDs []string `json:"expected_chunk_ids,omitempty"`
	ExpectedDocIDs   []string `json:"expected_doc_ids,omitempty"`
	// ValidAt / RecordedAt 是 G-05-asof 时态探针轴（RFC3339；缺省/空串 = 轴未设）。
	// 任一非空即要求被评检索器实现 TemporalSearcher 窄接口（runner 机会断言；
	// 缺端口记 CaseResult.Err，由黄金门禁 GateSearchError 卡红）。
	ValidAt    string `json:"valid_at,omitempty"`
	RecordedAt string `json:"recorded_at,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// temporalSpan 解析时态轴（两段 RFC3339 严格校验；空串 = 零值轴）。
// 解析失败语义与 case 检索失败同款——进 CaseResult.Err 而非 panic，
// 让门禁把「评测集与时间语法漂移」显式卡红。
func (gc GoldenCase) temporalSpan() (bizknowledge.TemporalSpan, error) {
	var span bizknowledge.TemporalSpan
	if raw := strings.TrimSpace(gc.ValidAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return span, fmt.Errorf("eval: case %q: valid_at must be RFC3339, got %q", gc.ID, gc.ValidAt)
		}
		span.ValidAt = t
	}
	if raw := strings.TrimSpace(gc.RecordedAt); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return span, fmt.Errorf("eval: case %q: recorded_at must be RFC3339, got %q", gc.ID, gc.RecordedAt)
		}
		span.RecordedAt = t
	}
	return span, nil
}

// EvalSet is a named collection of golden cases, loadable from disk.
type EvalSet struct {
	Name  string       `json:"name"`
	Cases []GoldenCase `json:"cases"`
}
