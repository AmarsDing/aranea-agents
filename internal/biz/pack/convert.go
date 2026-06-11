package pack

import (
	"sort"
)

// CollectPackDependencies 收集 Pack 中的 Skill 和 FuncRef 依赖。
// 导出供 data 层和 biz 层内部调用。
func CollectPackDependencies(p *Pack) {
	skillSet := make(map[string]bool)

	for _, a := range p.Agents {
		if a.Skills != nil {
			for _, s := range a.Skills.Allowed {
				skillSet[s] = true
			}
			for _, s := range a.Skills.Denied {
				skillSet[s] = true
			}
		}
	}

	if len(skillSet) > 0 {
		// 排序保证导出确定性（map 迭代顺序随机，会影响 Pack hash / 缓存）
		skills := make([]string, 0, len(skillSet))
		for s := range skillSet {
			skills = append(skills, s)
		}
		sort.Strings(skills)
		p.Manifest.Dependencies = &PackDependencies{
			Skills: skills,
		}
	}

	// 收集 contents
	p.Manifest.Contents = &PackContents{}
	if p.Organization != nil {
		p.Manifest.Contents.Organization = true
	}
	for _, a := range p.Agents {
		p.Manifest.Contents.Agents = append(p.Manifest.Contents.Agents, PackContentRef{Key: a.Key})
	}
	for _, t := range p.Teams {
		p.Manifest.Contents.Teams = append(p.Manifest.Contents.Teams, PackContentRef{Key: t.Key})
	}
	for _, g := range p.Graphs {
		p.Manifest.Contents.Graphs = append(p.Manifest.Contents.Graphs, PackContentRef{Key: g.ID})
	}
}

// ConvertGraphTemplatesToPack 将硬编码的 Graph 模板转换为 GraphPackSpec 列表。
// GraphTemplate 定义在 internal/graph/trpc 包中。
type GraphTemplateSource struct {
	ID          string
	Name        string
	Description string
	Category    string
	EntryPoint  string
	FinishPoint string
	Nodes       []TemplateNodeSource
	Edges       []TemplateEdgeSource
	StateFields []StateFieldSource
}

type TemplateNodeSource struct {
	NodeID      string
	Type        string
	Label       string
	Description string
}

type TemplateEdgeSource struct {
	FromNode string
	ToNode   string
	Type     string
	Label    string
}

type StateFieldSource struct {
	Name    string
	Type    string
	Reducer string
}

// ConvertGraphTemplates converts graph template sources to GraphPackSpec list.
func ConvertGraphTemplates(templates []GraphTemplateSource) []GraphPackSpec {
	var result []GraphPackSpec
	for _, t := range templates {
		gs := GraphPackSpec{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Category:    t.Category,
			EntryPoint:  t.EntryPoint,
			FinishPoint: t.FinishPoint,
		}

		for _, sf := range t.StateFields {
			gs.StateFields = append(gs.StateFields, StateFieldPackSpec{
				Name:    sf.Name,
				Type:    sf.Type,
				Reducer: sf.Reducer,
			})
		}

		for _, n := range t.Nodes {
			gs.Nodes = append(gs.Nodes, GraphNodePackSpec{
				ID:          n.NodeID,
				Type:        n.Type,
				Label:       n.Label,
				Description: n.Description,
			})
		}

		// 分离普通边和条件边
		var condEdgeMap = make(map[string]map[string]string) // from → label → to
		for _, e := range t.Edges {
			if e.Type == "conditional" {
				if condEdgeMap[e.FromNode] == nil {
					condEdgeMap[e.FromNode] = make(map[string]string)
				}
				label := e.Label
				if label == "" {
					label = e.ToNode
				}
				condEdgeMap[e.FromNode][label] = e.ToNode
			} else {
				gs.Edges = append(gs.Edges, GraphEdgePackSpec{
					From: e.FromNode,
					To:   e.ToNode,
				})
			}
		}

		for from, pathMap := range condEdgeMap {
			gs.ConditionalEdges = append(gs.ConditionalEdges, GraphCondEdgePackSpec{
				From:    from,
				PathMap: pathMap,
			})
		}

		result = append(result, gs)
	}
	return result
}

// MergePacks 将多个 Pack 合并为一个（用于将 taxonomy + agent templates + graph templates 合并）。
func MergePacks(packs ...*Pack) *Pack {
	result := &Pack{
		Manifest: ManifestSpec{
			APIVersion: "v1",
			Kind:       "industry",
			Version:    "1.0.0",
			Author:     "system",
		},
		AgentFiles: make(map[string]map[string]string),
	}

	for _, p := range packs {
		if p.Organization != nil {
			result.Organization = p.Organization
		}
		result.Agents = append(result.Agents, p.Agents...)
		result.Teams = append(result.Teams, p.Teams...)
		result.Graphs = append(result.Graphs, p.Graphs...)
		for k, v := range p.AgentFiles {
			result.AgentFiles[k] = v
		}
	}

	// 使用第一个有名称的 Pack 的名称
	for _, p := range packs {
		if p.Manifest.Name != "" {
			result.Manifest.Name = p.Manifest.Name
			result.Manifest.Description = p.Manifest.Description
			break
		}
	}

	CollectPackDependencies(result)
	return result
}
