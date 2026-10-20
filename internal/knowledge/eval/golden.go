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

// GoldenCase is one curated expectation for a retrieval query. Expected ids
// may be given at chunk granularity, document granularity, or both; the
// runner scores both bases independently. An empty expected list means
// "nothing to recall" and every metric returns 1.0 (vacuous success).
type GoldenCase struct {
	ID               string   `json:"id"`
	Query            string   `json:"query"`
	ExpectedChunkIDs []string `json:"expected_chunk_ids,omitempty"`
	ExpectedDocIDs   []string `json:"expected_doc_ids,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

// EvalSet is a named collection of golden cases, loadable from disk.
type EvalSet struct {
	Name  string       `json:"name"`
	Cases []GoldenCase `json:"cases"`
}
