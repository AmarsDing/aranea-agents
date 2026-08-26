package configgraph

import "testing"

func grantedToolEdge(origin string) StoredEdge {
	return StoredEdge{
		SrcID: "a1", DstID: "t1", Type: EdgeTypeGrantedTool,
		Evidence: map[string]any{EvidenceKeyGrantOrigin: origin},
	}
}

// TestEdgeWeight 锁定 design §5.1 的边权重分档：override 30 / allow 15 /
// profile 2 / 其他 5；origin 缺失兜底按显式 allow。
func TestEdgeWeight(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edge StoredEdge
		want int
	}{
		{"tool_override 边 30", StoredEdge{Type: EdgeTypeToolOverride}, 30},
		{"granted_tool override 30", grantedToolEdge(GrantOriginOverride), 30},
		{"granted_tool allow 15", grantedToolEdge(GrantOriginAllow), 15},
		{"granted_tool profile 2", grantedToolEdge(GrantOriginProfile), 2},
		{"granted_tool 无 origin 兜底 15", StoredEdge{Type: EdgeTypeGrantedTool}, 15},
		{"has_member 5", StoredEdge{Type: EdgeTypeHasMember}, 5},
		{"runs 5", StoredEdge{Type: EdgeTypeRuns}, 5},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := edgeWeight(tc.edge); got != tc.want {
				t.Errorf("edgeWeight = %d, want %d", got, tc.want)
			}
		})
	}
}

func toolTarget(riskLevel string) Node {
	return Node{ID: "t1", NodeType: NodeTypeTool, RefID: "t1", NodeKey: "shell",
		Attrs: map[string]any{"risk_level": riskLevel}}
}

// TestRiskScore_ThreeTiers 构造三档位用例断言（dev-plan 1.2 验收）。
func TestRiskScore_ThreeTiers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		target  Node
		edges   []StoredEdge
		signals ImpactSignals
		wantMin int
		wantMax int
		level   string
	}{
		{"low：仅 profile 隐式", toolTarget("low"),
			[]StoredEdge{grantedToolEdge(GrantOriginProfile), grantedToolEdge(GrantOriginProfile)},
			ImpactSignals{}, 4, 4, RiskLevelLow},
		{"medium：两个 allow 显式", toolTarget("low"),
			[]StoredEdge{grantedToolEdge(GrantOriginAllow), grantedToolEdge(GrantOriginAllow)},
			ImpactSignals{}, 30, 30, RiskLevelMedium},
		{"high：两个 override + 高危工具目标", toolTarget("high"),
			[]StoredEdge{grantedToolEdge(GrantOriginOverride), grantedToolEdge(GrantOriginOverride)},
			ImpactSignals{}, 80, 80, RiskLevelHigh},
		{"边界 29 low", toolTarget("low"),
			[]StoredEdge{grantedToolEdge(GrantOriginAllow), grantedToolEdge(GrantOriginProfile),
				{Type: EdgeTypeHasMember}},
			ImpactSignals{ActiveSessions: 7}, 29, 29, RiskLevelLow},
		{"边界 60 high", toolTarget("low"),
			[]StoredEdge{grantedToolEdge(GrantOriginOverride), grantedToolEdge(GrantOriginAllow),
				grantedToolEdge(GrantOriginAllow)},
			ImpactSignals{}, 60, 60, RiskLevelHigh},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := RiskScore(tc.target, tc.edges, tc.signals)
			if r.Score < tc.wantMin || r.Score > tc.wantMax {
				t.Errorf("score = %d, want [%d,%d]", r.Score, tc.wantMin, tc.wantMax)
			}
			if r.Level != tc.level {
				t.Errorf("level = %s, want %s (score %d)", r.Level, tc.level, r.Score)
			}
		})
	}
}

// TestRiskScore_ProfileNoiseNotHigh dev-plan 1.2 验收：基础工具（profile
// 隐式引用居多）变更不触 high——即便叠加默认团队/会话加成。
func TestRiskScore_ProfileNoiseNotHigh(t *testing.T) {
	t.Parallel()
	edges := make([]StoredEdge, 0, 10)
	for i := 0; i < 10; i++ {
		edges = append(edges, grantedToolEdge(GrantOriginProfile)) // 10×2=20
	}
	r := RiskScore(toolTarget("low"), edges, ImpactSignals{DefaultTeam: true, ActiveSessions: 5})
	// 20 + 15 + 5 = 40 → medium，绝不触 high。
	if r.Level == RiskLevelHigh {
		t.Fatalf("profile-noise case must not reach high, got score %d", r.Score)
	}
	if r.Score != 40 {
		t.Errorf("score = %d, want 40", r.Score)
	}
}

// TestRiskScore_Bonuses 加成规则逐项断言。
func TestRiskScore_Bonuses(t *testing.T) {
	t.Parallel()

	t.Run("高危工具目标 +20", func(t *testing.T) {
		r := RiskScore(toolTarget("high"), nil, ImpactSignals{})
		if r.Score != 20 || !r.Breakdown.HighRiskTarget {
			t.Errorf("got %+v", r)
		}
	})
	t.Run("非工具目标不加", func(t *testing.T) {
		r := RiskScore(Node{NodeType: NodeTypeAgent}, nil, ImpactSignals{})
		if r.Score != 0 || r.Breakdown.HighRiskTarget {
			t.Errorf("got %+v", r)
		}
	})
	t.Run("默认团队 +15", func(t *testing.T) {
		r := RiskScore(toolTarget("low"), nil, ImpactSignals{DefaultTeam: true})
		if r.Score != 15 || !r.Breakdown.DefaultTeamHit {
			t.Errorf("got %+v", r)
		}
	})
	t.Run("cron 命中 +10", func(t *testing.T) {
		r := RiskScore(toolTarget("low"), nil, ImpactSignals{CronTasks: 2})
		if r.Score != 10 || !r.Breakdown.CronHit {
			t.Errorf("got %+v", r)
		}
	})
	t.Run("活跃会话 +1/个 封顶 10", func(t *testing.T) {
		r := RiskScore(toolTarget("low"), nil, ImpactSignals{ActiveSessions: 25})
		if r.Score != 10 || r.Breakdown.SessionBonus != 10 {
			t.Errorf("got %+v", r)
		}
	})
	t.Run("分项计数", func(t *testing.T) {
		edges := []StoredEdge{
			grantedToolEdge(GrantOriginOverride),
			grantedToolEdge(GrantOriginAllow),
			grantedToolEdge(GrantOriginProfile),
			{Type: EdgeTypeHasMember},
		}
		r := RiskScore(toolTarget("low"), edges, ImpactSignals{})
		bd := r.Breakdown
		if bd.OverrideEdges != 1 || bd.AllowEdges != 1 || bd.ProfileEdges != 1 || bd.OtherEdges != 1 {
			t.Errorf("breakdown = %+v", bd)
		}
		if bd.EdgeScore != 30+15+2+5 {
			t.Errorf("edge_score = %d, want 52", bd.EdgeScore)
		}
	})
}
