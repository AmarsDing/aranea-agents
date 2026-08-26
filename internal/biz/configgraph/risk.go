package configgraph

// Risk 加权分（design 81-config-graph.design.md §5.1）：
//
//	score = Σ(闭包内命中边权重) + 加成
//	  override 引用        30/个（tool_override 边 或 granted_tool{origin=override}）
//	  allow 显式           15/个（granted_tool{origin=allow}）
//	  profile 隐式          2/个（防基础工具告警疲劳）
//	  其他边（has_member 等） 5/个
//	加成：
//	  变更对象为 high risk_level 工具  +20
//	  命中默认团队                     +15
//	  命中 cron_task 节点              +10（v1 简化：不判运行态）
//	  活跃会话                         +1/个，封顶 10
//	level：score ≥ 60 → high；30–59 → medium；< 30 → low

// Risk levels for impact blast radius (design §5.1).
const (
	RiskLevelLow    = "low"
	RiskLevelMedium = "medium"
	RiskLevelHigh   = "high"
)

const (
	riskThresholdHigh   = 60
	riskThresholdMedium = 30
)

const (
	riskBonusHighRiskTool = 20
	riskBonusDefaultTeam  = 15
	riskBonusCronHit      = 10
	riskSessionBonusCap   = 10
)

const (
	edgeWeightOverride = 30
	edgeWeightAllow    = 15
	edgeWeightProfile  = 2
	edgeWeightOther    = 5
)

// edgeWeight returns the per-edge risk weight (design §5.1). granted_tool 边
// 按 evidence.grant_origin 分档；origin 缺失兜底按显式 allow（P0 抽取侧必
// 标 origin，缺失仅属防御分支）。
func edgeWeight(e StoredEdge) int {
	switch e.Type {
	case EdgeTypeToolOverride:
		return edgeWeightOverride
	case EdgeTypeGrantedTool:
		switch grantOriginOf(e.Evidence) {
		case GrantOriginOverride:
			return edgeWeightOverride
		case GrantOriginProfile:
			return edgeWeightProfile
		default: // GrantOriginAllow / 缺失兜底
			return edgeWeightAllow
		}
	default:
		return edgeWeightOther
	}
}

func grantOriginOf(ev map[string]any) string {
	s, _ := ev[EvidenceKeyGrantOrigin].(string)
	return s
}

// RiskBreakdown 记录加权分项（可解释性：为什么是这个分）。
type RiskBreakdown struct {
	OverrideEdges  int  `json:"override_edges"`
	AllowEdges     int  `json:"allow_edges"`
	ProfileEdges   int  `json:"profile_edges"`
	OtherEdges     int  `json:"other_edges"`
	EdgeScore      int  `json:"edge_score"`
	HighRiskTarget bool `json:"high_risk_target"` // +20
	DefaultTeamHit bool `json:"default_team_hit"` // +15
	CronHit        bool `json:"cron_hit"`         // +10
	SessionBonus   int  `json:"session_bonus"`    // +1/个 封顶 10
}

// Risk 是影响面加权分结论。
type Risk struct {
	Score     int           `json:"score"`
	Level     string        `json:"level"`
	Breakdown RiskBreakdown `json:"breakdown"`
}

// riskLevel maps score to level (≥60 high；30–59 medium；<30 low).
func riskLevel(score int) string {
	switch {
	case score >= riskThresholdHigh:
		return RiskLevelHigh
	case score >= riskThresholdMedium:
		return RiskLevelMedium
	default:
		return RiskLevelLow
	}
}

// RiskScore computes the weighted blast-radius risk over one impact closure
// (design §5.1)。edges 为闭包内命中边（调用方已按边身份去重）；target 为
// 变更对象本体。
func RiskScore(target Node, edges []StoredEdge, signals ImpactSignals) Risk {
	var bd RiskBreakdown
	for _, e := range edges {
		w := edgeWeight(e)
		bd.EdgeScore += w
		switch w {
		case edgeWeightOverride:
			bd.OverrideEdges++
		case edgeWeightAllow:
			bd.AllowEdges++
		case edgeWeightProfile:
			bd.ProfileEdges++
		default:
			bd.OtherEdges++
		}
	}
	score := bd.EdgeScore
	if target.NodeType == NodeTypeTool {
		if rl, _ := target.Attrs["risk_level"].(string); rl == "high" {
			bd.HighRiskTarget = true
			score += riskBonusHighRiskTool
		}
	}
	if signals.DefaultTeam {
		bd.DefaultTeamHit = true
		score += riskBonusDefaultTeam
	}
	if signals.CronTasks > 0 {
		bd.CronHit = true
		score += riskBonusCronHit
	}
	bd.SessionBonus = min(int(signals.ActiveSessions), riskSessionBonusCap)
	score += bd.SessionBonus
	return Risk{Score: score, Level: riskLevel(score), Breakdown: bd}
}
