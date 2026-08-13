package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// toolQualityStatsRepo 按工具聚合 tool_invocations 的调用质量指标
// （29-token 工具质量度量）。Raw SQL：聚合 + metadata_json JSON 提取
// 超出 Ent API 表达能力，按 DB-N2 走事务感知 Raw SQL 读路径。
type toolQualityStatsRepo struct {
	data *Data
}

// NewToolQualityStatsRepo 创建工具质量聚合查询 Repo。
func NewToolQualityStatsRepo(d *Data) biz.ToolQualityStatsReader {
	return &toolQualityStatsRepo{data: d}
}

// GetToolQualityStats 按工具聚合 since 之后的调用质量；agentID 为空表示全部 agent。
//
// 参数质量标记由 repair guard 写入 metadata_json（args_repaired / args_invalid）：
//   - ArgsFirstPassRate = 1 - (repaired+invalid)/count —— 参数一次合法率，
//     衡量模型对该工具 schema 的一次性理解准确度。
//   - blocked（待用户确认）不计入失败，它不代表工具执行错误。
func (r *toolQualityStatsRepo) GetToolQualityStats(ctx context.Context, agentID string, since time.Time) ([]biz.ToolQualityStat, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal("TOOL", "raw db unavailable")
	}
	repairedExpr := r.data.Dialect().JSONExtract("ti.metadata_json", "args_repaired")
	invalidExpr := r.data.Dialect().JSONExtract("ti.metadata_json", "args_invalid")
	// SQLite json_extract 对 JSON true 返回整数 1；Postgres ->> 返回文本 'true'。
	trueLiteral := "'true'"
	if r.data.Dialect().IsSQLite() {
		trueLiteral = "1"
	}
	q := `
SELECT ti.tool_key,
       COUNT(*) AS cnt,
       SUM(CASE WHEN ti.status = 'success' THEN 1 ELSE 0 END) AS success_cnt,
       SUM(CASE WHEN ti.status IN ('failed', 'error', 'cancelled') THEN 1 ELSE 0 END) AS failure_cnt,
       SUM(CASE WHEN ` + repairedExpr + ` = ` + trueLiteral + ` THEN 1 ELSE 0 END) AS repaired_cnt,
       SUM(CASE WHEN ` + invalidExpr + ` = ` + trueLiteral + ` THEN 1 ELSE 0 END) AS invalid_cnt,
       AVG(ti.duration_ms) AS avg_duration_ms
FROM tool_invocations ti
WHERE ti.started_at >= ? AND COALESCE(ti.deleted_at, '') = ''`
	args := []any{since.UTC().Format(time.RFC3339)}
	if strings.TrimSpace(agentID) != "" {
		q += ` AND ti.agent_id = ?`
		args = append(args, strings.TrimSpace(agentID))
	}
	q += ` GROUP BY ti.tool_key ORDER BY cnt DESC, ti.tool_key`

	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()

	var out []biz.ToolQualityStat
	for rows.Next() {
		var s biz.ToolQualityStat
		var avgMs float64
		if err := rows.Scan(&s.ToolKey, &s.Count, &s.SuccessCount, &s.FailureCount, &s.RepairedCount, &s.InvalidCount, &avgMs); err != nil {
			return nil, entErrToBizErr(err, "TOOL")
		}
		s.AvgDurationMs = int(avgMs)
		if s.Count > 0 {
			s.SuccessRate = float64(s.SuccessCount) / float64(s.Count)
			s.ArgsFirstPassRate = 1 - float64(s.RepairedCount+s.InvalidCount)/float64(s.Count)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	return out, nil
}
