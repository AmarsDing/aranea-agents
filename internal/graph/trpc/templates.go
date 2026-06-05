package graph

import (
	"aranea-agents/internal/biz"
)

type GraphTemplate struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Nodes       []TemplateNode  `json:"nodes"`
	Edges       []TemplateEdge  `json:"edges"`
	StateFields []StateFieldDef `json:"state_fields"`
	EntryPoint  string          `json:"entry_point"`
	FinishPoint string          `json:"finish_point"`
}

type TemplateNode struct {
	NodeID      string `json:"node_id"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type TemplateEdge struct {
	FromNode string `json:"from_node"`
	ToNode   string `json:"to_node"`
	Type     string `json:"type"`
	Label    string `json:"label,omitempty"`
}

var builtinTemplates = []GraphTemplate{
	{
		ID:          "pipeline",
		Name:        "顺序流水线",
		Description: "数据按顺序经过多个处理阶段，适用于数据处理管线、报告生成等场景",
		Category:    "pipeline",
		EntryPoint:  "step_1",
		FinishPoint: "step_4",
		Nodes: []TemplateNode{
			{NodeID: "step_1", Type: "function", Label: "步骤1", Description: "第一个处理阶段"},
			{NodeID: "step_2", Type: "function", Label: "步骤2", Description: "第二个处理阶段"},
			{NodeID: "step_3", Type: "function", Label: "步骤3", Description: "第三个处理阶段"},
			{NodeID: "step_4", Type: "function", Label: "步骤4", Description: "最终处理阶段"},
		},
		Edges: []TemplateEdge{
			{FromNode: "step_1", ToNode: "step_2", Type: "runtime"},
			{FromNode: "step_2", ToNode: "step_3", Type: "runtime"},
			{FromNode: "step_3", ToNode: "step_4", Type: "runtime"},
		},
		StateFields: []StateFieldDef{
			{Name: "input", Type: "string", Reducer: ReducerDefault},
			{Name: "output", Type: "string", Reducer: ReducerDefault},
			{Name: "messages", Type: "[]any", Reducer: ReducerAppend},
		},
	},
	{
		ID:          "approval",
		Name:        "审批流",
		Description: "包含人工审批节点的流程，适用于需要人工确认的业务场景",
		Category:    "approval",
		EntryPoint:  "submit",
		FinishPoint: "result",
		Nodes: []TemplateNode{
			{NodeID: "submit", Type: "function", Label: "提交申请", Description: "提交处理申请"},
			{NodeID: "review", Type: "function", Label: "审批", Description: "人工审批确认节点"},
			{NodeID: "approved", Type: "function", Label: "审批通过", Description: "审批通过后执行"},
			{NodeID: "rejected", Type: "function", Label: "审批驳回", Description: "审批驳回后处理"},
			{NodeID: "result", Type: "function", Label: "结果", Description: "最终结果汇总"},
		},
		Edges: []TemplateEdge{
			{FromNode: "submit", ToNode: "review", Type: "runtime"},
			{FromNode: "review", ToNode: "approved", Type: "conditional", Label: "approved"},
			{FromNode: "review", ToNode: "rejected", Type: "conditional", Label: "rejected"},
			{FromNode: "approved", ToNode: "result", Type: "runtime"},
			{FromNode: "rejected", ToNode: "result", Type: "runtime"},
		},
		StateFields: []StateFieldDef{
			{Name: "input", Type: "string", Reducer: ReducerDefault},
			{Name: "status", Type: "string", Reducer: ReducerDefault},
			{Name: "review_result", Type: "string", Reducer: ReducerDefault},
		},
	},
	{
		ID:          "parallel_review",
		Name:        "并行评审",
		Description: "多个评审者并行执行评审任务，最后汇总结果",
		Category:    "parallel",
		EntryPoint:  "dispatch",
		FinishPoint: "aggregate",
		Nodes: []TemplateNode{
			{NodeID: "dispatch", Type: "function", Label: "分发任务", Description: "将任务分发给多个评审者"},
			{NodeID: "reviewer_a", Type: "function", Label: "评审者A", Description: "评审者A的评审"},
			{NodeID: "reviewer_b", Type: "function", Label: "评审者B", Description: "评审者B的评审"},
			{NodeID: "reviewer_c", Type: "function", Label: "评审者C", Description: "评审者C的评审"},
			{NodeID: "aggregate", Type: "function", Label: "汇总", Description: "汇总所有评审结果"},
		},
		Edges: []TemplateEdge{
			{FromNode: "dispatch", ToNode: "reviewer_a", Type: "runtime"},
			{FromNode: "dispatch", ToNode: "reviewer_b", Type: "runtime"},
			{FromNode: "dispatch", ToNode: "reviewer_c", Type: "runtime"},
			{FromNode: "reviewer_a", ToNode: "aggregate", Type: "runtime"},
			{FromNode: "reviewer_b", ToNode: "aggregate", Type: "runtime"},
			{FromNode: "reviewer_c", ToNode: "aggregate", Type: "runtime"},
		},
		StateFields: []StateFieldDef{
			{Name: "input", Type: "string", Reducer: ReducerDefault},
			{Name: "reviews", Type: "[]any", Reducer: ReducerAppend},
			{Name: "summary", Type: "string", Reducer: ReducerDefault},
		},
	},
	{
		ID:          "review_loop",
		Name:        "生成-评审循环",
		Description: "生成内容后评审打分，不达标则循环修改，适用于迭代优化场景",
		Category:    "loop",
		EntryPoint:  "generate",
		FinishPoint: "final",
		Nodes: []TemplateNode{
			{NodeID: "generate", Type: "function", Label: "生成", Description: "生成或修改内容"},
			{NodeID: "evaluate", Type: "function", Label: "评审", Description: "评审打分"},
			{NodeID: "route", Type: "router", Label: "质量判断", Description: "根据评分决定是否继续迭代"},
			{NodeID: "final", Type: "function", Label: "最终输出", Description: "输出最终结果"},
		},
		Edges: []TemplateEdge{
			{FromNode: "generate", ToNode: "evaluate", Type: "runtime"},
			{FromNode: "evaluate", ToNode: "route", Type: "runtime"},
			{FromNode: "route", ToNode: "final", Type: "conditional", Label: "approved"},
			{FromNode: "route", ToNode: "generate", Type: "conditional", Label: "revision"},
		},
		StateFields: []StateFieldDef{
			{Name: "input", Type: "string", Reducer: ReducerDefault},
			{Name: "output", Type: "string", Reducer: ReducerDefault},
			{Name: "score", Type: "float64", Reducer: ReducerDefault},
			{Name: "iteration", Type: "int", Reducer: ReducerDefault},
		},
	},
	{
		ID:          "dispatch",
		Name:        "条件分发",
		Description: "根据条件将任务分发到不同的处理分支，适用于分类处理场景",
		Category:    "dispatch",
		EntryPoint:  "classify",
		FinishPoint: "done",
		Nodes: []TemplateNode{
			{NodeID: "classify", Type: "function", Label: "分类", Description: "分析并分类输入"},
			{NodeID: "router", Type: "router", Label: "路由", Description: "根据分类结果路由"},
			{NodeID: "handler_a", Type: "function", Label: "处理A", Description: "处理A类任务"},
			{NodeID: "handler_b", Type: "function", Label: "处理B", Description: "处理B类任务"},
			{NodeID: "handler_c", Type: "function", Label: "处理C", Description: "处理C类任务"},
			{NodeID: "done", Type: "function", Label: "完成", Description: "汇总结果"},
		},
		Edges: []TemplateEdge{
			{FromNode: "classify", ToNode: "router", Type: "runtime"},
			{FromNode: "router", ToNode: "handler_a", Type: "conditional", Label: "type_a"},
			{FromNode: "router", ToNode: "handler_b", Type: "conditional", Label: "type_b"},
			{FromNode: "router", ToNode: "handler_c", Type: "conditional", Label: "type_c"},
			{FromNode: "handler_a", ToNode: "done", Type: "runtime"},
			{FromNode: "handler_b", ToNode: "done", Type: "runtime"},
			{FromNode: "handler_c", ToNode: "done", Type: "runtime"},
		},
		StateFields: []StateFieldDef{
			{Name: "input", Type: "string", Reducer: ReducerDefault},
			{Name: "category", Type: "string", Reducer: ReducerDefault},
			{Name: "result", Type: "string", Reducer: ReducerDefault},
		},
	},
	{
		ID:          "nested_subgraph",
		Name:        "子图嵌套",
		Description: "包含子工作流的嵌套结构，适用于复用通用流程片段",
		Category:    "nested",
		EntryPoint:  "preprocess",
		FinishPoint: "postprocess",
		Nodes: []TemplateNode{
			{NodeID: "preprocess", Type: "function", Label: "预处理", Description: "数据预处理"},
			{NodeID: "sub_workflow", Type: "agent", Label: "子工作流", Description: "嵌套的子工作流"},
			{NodeID: "postprocess", Type: "function", Label: "后处理", Description: "结果后处理"},
		},
		Edges: []TemplateEdge{
			{FromNode: "preprocess", ToNode: "sub_workflow", Type: "runtime"},
			{FromNode: "sub_workflow", ToNode: "postprocess", Type: "runtime"},
		},
		StateFields: []StateFieldDef{
			{Name: "input", Type: "string", Reducer: ReducerDefault},
			{Name: "intermediate", Type: "string", Reducer: ReducerDefault},
			{Name: "output", Type: "string", Reducer: ReducerDefault},
		},
	},
}

func ListBuiltinTemplates() []GraphTemplate {
	return builtinTemplates
}

func GetBuiltinTemplate(id string) *GraphTemplate {
	for i := range builtinTemplates {
		if builtinTemplates[i].ID == id {
			return &builtinTemplates[i]
		}
	}
	return nil
}

func TemplateToBuildConfig(tmpl GraphTemplate) GraphBuildConfig {
	nodes := make([]biz.NodeDef, len(tmpl.Nodes))
	for i, tn := range tmpl.Nodes {
		nodes[i] = biz.NodeDef{
			ID:          tn.NodeID,
			Type:        tn.Type,
			Description: tn.Description,
		}
	}

	edges := make([]EdgeDef, 0)
	var condEdges []biz.ConditionalEdgeDef

	routePathMaps := make(map[string]map[string]string)

	for _, te := range tmpl.Edges {
		if te.Type == "conditional" {
			if _, ok := routePathMaps[te.FromNode]; !ok {
				routePathMaps[te.FromNode] = make(map[string]string)
			}
			label := te.Label
			if label == "" {
				label = te.ToNode
			}
			routePathMaps[te.FromNode][label] = te.ToNode
		} else {
			edges = append(edges, EdgeDef{From: te.FromNode, To: te.ToNode})
		}
	}

	for from, pathMap := range routePathMaps {
		condEdges = append(condEdges, biz.ConditionalEdgeDef{
			From:    from,
			PathMap: pathMap,
		})
	}

	stateFields := make([]StateFieldDef, len(tmpl.StateFields))
	copy(stateFields, tmpl.StateFields)

	return GraphBuildConfig{
		Nodes:            nodes,
		Edges:            edges,
		ConditionalEdges: condEdges,
		StateFields:      stateFields,
		EntryPoint:       tmpl.EntryPoint,
		FinishPoint:      tmpl.FinishPoint,
	}
}
