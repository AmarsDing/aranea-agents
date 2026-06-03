package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestPackRoundTrip(t *testing.T) {
	original := &Pack{
		Manifest: ManifestSpec{
			APIVersion: "v1",
			Kind:       "agent",
			Name:       "测试 Agent",
			Version:    "1.0.0",
		},
		Taxonomy: &TaxonomyPackSpec{
			Industries: []IndustrySpec{
				{
					Key:   "finance",
					Name:  "金融",
					Icon:  "trending_up",
					SortOrder: 1,
					Departments: []DepartmentSpec{
						{
							Key:   "quant_trading",
							Name:  "量化交易",
							SortOrder: 1,
							Positions: []PositionSpec{
								{Key: "quant_researcher", Name: "量化研究员", SortOrder: 1},
							},
						},
					},
				},
			},
		},
		Agents: []AgentPackSpec{
			{
				Key:         "go-senior-general",
				DisplayName: "Go 高级工程师",
				Description: "Go 后端通用开发",
				Provider:    "openrouter",
				Model:       "gpt-4.1-mini",
				PositionKey: "finance/quant_trading/quant_researcher",
				Variant:     "general",
				Files: []AgentFileRef{
					{Name: "IDENTITY.md"},
					{Name: "SOUL.md"},
				},
			},
		},
		Teams: []TeamPackSpec{
			{
				Key:         "team-fullstack",
				DisplayName: "全栈功能开发团队",
				Mode:        "coordinator",
				Members: []TeamMemberPackSpec{
					{AgentKey: "go-senior-general", Role: "member"},
				},
			},
		},
		Graphs: []GraphPackSpec{
			{
				ID:          "pipeline",
				Name:        "顺序流水线",
				Description: "数据按顺序经过多个处理阶段",
				Category:    "pipeline",
				EntryPoint:  "step_1",
				FinishPoint: "step_4",
			},
		},
		AgentFiles: map[string]map[string]string{
			"go-senior-general": {
				"IDENTITY.md": "# Identity\n你是一个 Go 高级工程师。",
				"SOUL.md":     "# Soul\n你热爱简洁的代码。",
			},
		},
	}

	// 写出
	var buf bytes.Buffer
	if err := WritePack(original, &buf); err != nil {
		t.Fatalf("WritePack 失败: %v", err)
	}

	// 读回
	got, err := ReadPack(&buf)
	if err != nil {
		t.Fatalf("ReadPack 失败: %v", err)
	}

	// 验证 manifest
	if got.Manifest.APIVersion != "v1" {
		t.Errorf("APIVersion = %q, want %q", got.Manifest.APIVersion, "v1")
	}
	if got.Manifest.Kind != "agent" {
		t.Errorf("Kind = %q, want %q", got.Manifest.Kind, "agent")
	}
	if got.Manifest.Name != "测试 Agent" {
		t.Errorf("Name = %q, want %q", got.Manifest.Name, "测试 Agent")
	}

	// 验证 taxonomy
	if got.Taxonomy == nil {
		t.Fatal("Taxonomy 为 nil")
	}
	if len(got.Taxonomy.Industries) != 1 {
		t.Fatalf("Industries 数量 = %d, want 1", len(got.Taxonomy.Industries))
	}
	if got.Taxonomy.Industries[0].Key != "finance" {
		t.Errorf("Industry Key = %q, want %q", got.Taxonomy.Industries[0].Key, "finance")
	}

	// 验证 agents
	if len(got.Agents) != 1 {
		t.Fatalf("Agents 数量 = %d, want 1", len(got.Agents))
	}
	if got.Agents[0].Key != "go-senior-general" {
		t.Errorf("Agent Key = %q, want %q", got.Agents[0].Key, "go-senior-general")
	}
	if got.Agents[0].PositionKey != "finance/quant_trading/quant_researcher" {
		t.Errorf("PositionKey = %q, want %q", got.Agents[0].PositionKey, "finance/quant_trading/quant_researcher")
	}

	// 验证 agent files
	if len(got.AgentFiles["go-senior-general"]) != 2 {
		t.Fatalf("AgentFiles 数量 = %d, want 2", len(got.AgentFiles["go-senior-general"]))
	}
	if got.AgentFiles["go-senior-general"]["IDENTITY.md"] != "# Identity\n你是一个 Go 高级工程师。" {
		t.Errorf("IDENTITY.md 内容不匹配")
	}

	// 验证 teams
	if len(got.Teams) != 1 {
		t.Fatalf("Teams 数量 = %d, want 1", len(got.Teams))
	}
	if got.Teams[0].Key != "team-fullstack" {
		t.Errorf("Team Key = %q, want %q", got.Teams[0].Key, "team-fullstack")
	}
	if len(got.Teams[0].Members) != 1 || got.Teams[0].Members[0].AgentKey != "go-senior-general" {
		t.Errorf("Team Members 不匹配")
	}

	// 验证 graphs
	if len(got.Graphs) != 1 {
		t.Fatalf("Graphs 数量 = %d, want 1", len(got.Graphs))
	}
	if got.Graphs[0].ID != "pipeline" {
		t.Errorf("Graph ID = %q, want %q", got.Graphs[0].ID, "pipeline")
	}
}

func TestReadPackInvalidGzip(t *testing.T) {
	_, err := ReadPack(bytes.NewReader([]byte("not a gzip")))
	if err == nil {
		t.Fatal("期望错误，但未返回")
	}
}

func TestReadPackMissingManifest(t *testing.T) {
	// 创建一个不包含 manifest.yaml 的 tar.gz
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	tw.Close()
	gzw.Close()

	_, err := ReadPack(&buf)
	if err == nil {
		t.Fatal("期望错误，但未返回")
	}
}

func TestKeyMapper(t *testing.T) {
	m := NewKeyMapper()

	// Agent 映射
	m.RegisterAgent("go-senior-general", "agent-123")
	id, ok := m.AgentID("go-senior-general")
	if !ok || id != "agent-123" {
		t.Errorf("AgentID = %q, %v; want %q, true", id, ok, "agent-123")
	}

	// Taxonomy 映射
	m.RegisterTaxonomy("finance/quant_trading/quant_researcher", "tax-pos-456")
	tid, err := m.ResolvePositionKey("finance/quant_trading/quant_researcher")
	if err != nil || tid != "tax-pos-456" {
		t.Errorf("ResolvePositionKey = %q, %v; want %q, nil", tid, err, "tax-pos-456")
	}

	// 未找到
	_, err = m.ResolveAgentKey("nonexistent")
	if err == nil {
		t.Fatal("期望错误，但未返回")
	}

	// Graph 映射
	m.RegisterGraph("pipeline", "graph-789")
	gid, ok := m.GraphID("pipeline")
	if !ok || gid != "graph-789" {
		t.Errorf("GraphID = %q, %v; want %q, true", gid, ok, "graph-789")
	}
}

func TestParseTaxonomyKeyPath(t *testing.T) {
	tests := []struct {
		input   string
		ind     string
		dept    string
		pos     string
		wantErr bool
	}{
		{"finance", "finance", "", "", false},
		{"finance/quant_trading", "finance", "quant_trading", "", false},
		{"finance/quant_trading/quant_researcher", "finance", "quant_trading", "quant_researcher", false},
		{"a/b/c/d", "", "", "", true},
	}
	for _, tt := range tests {
		ind, dept, pos, err := ParseTaxonomyKeyPath(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseTaxonomyKeyPath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
		}
		if !tt.wantErr {
			if ind != tt.ind || dept != tt.dept || pos != tt.pos {
				t.Errorf("ParseTaxonomyKeyPath(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.input, ind, dept, pos, tt.ind, tt.dept, tt.pos)
			}
		}
	}
}
