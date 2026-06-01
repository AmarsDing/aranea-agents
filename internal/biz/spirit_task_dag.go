package biz

import (
	"encoding/json"
	"fmt"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

type TaskNodeID string

type TaskNode struct {
	ID          TaskNodeID   `json:"id"`
	TaskName    string       `json:"task_name"`
	Description string       `json:"description"`
	DependsOn   []TaskNodeID `json:"depends_on"`
	Mode        string       `json:"mode"`
	AgentKeys   []string     `json:"agent_keys"`
}

type TaskDAG struct {
	Nodes map[TaskNodeID]*TaskNode `json:"nodes"`
	Roots []TaskNodeID             `json:"roots"`
}

func NewTaskDAG(nodes []TaskNode) (*TaskDAG, error) {
	dag := &TaskDAG{
		Nodes: make(map[TaskNodeID]*TaskNode, len(nodes)),
	}
	for i := range nodes {
		id := nodes[i].ID
		if id == "" {
			return nil, kerrors.BadRequest("SPIRIT", "task node id is required")
		}
		if _, exists := dag.Nodes[id]; exists {
			return nil, kerrors.BadRequest("SPIRIT", fmt.Sprintf("duplicate task node id: %s", id))
		}
		dag.Nodes[id] = &nodes[i]
	}
	if err := dag.validateDependencies(); err != nil {
		return nil, err
	}
	if err := dag.detectCycles(); err != nil {
		return nil, err
	}
	dag.Roots = dag.computeRoots()
	return dag, nil
}

func ParseTaskDAG(jsonStr string, lg loggateway.Logger) (*TaskDAG, error) {
	if jsonStr == "" {
		return nil, nil
	}
	var nodes []TaskNode
	if err := json.Unmarshal([]byte(jsonStr), &nodes); err != nil {
		lg.Warn("解析 task dag json 失败", loggateway.StepID("spirit.task_dag"), loggateway.Err(err))
		return nil, kerrors.BadRequest("SPIRIT", "invalid task dag json: "+err.Error())
	}
	return NewTaskDAG(nodes)
}

func (d *TaskDAG) ToJSON() (string, error) {
	if d == nil || len(d.Nodes) == 0 {
		return "", nil
	}
	nodes := d.OrderedNodes()
	b, err := json.Marshal(nodes)
	if err != nil {
		return "", kerrors.InternalServer("SPIRIT", "marshal task dag: "+err.Error())
	}
	return string(b), nil
}

func (d *TaskDAG) OrderedNodes() []*TaskNode {
	if d == nil {
		return nil
	}
	seen := make(map[TaskNodeID]bool, len(d.Nodes))
	var result []*TaskNode
	var visit func(ids []TaskNodeID)
	visit = func(ids []TaskNodeID) {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			node := d.Nodes[id]
			if node != nil {
				visit(node.DependsOn)
				result = append(result, node)
			}
		}
	}
	visit(d.Roots)
	for id, node := range d.Nodes {
		if !seen[id] {
			result = append(result, node)
		}
	}
	return result
}

func (d *TaskDAG) validateDependencies() error {
	for id, node := range d.Nodes {
		for _, depID := range node.DependsOn {
			if _, exists := d.Nodes[depID]; !exists {
				return kerrors.BadRequest("SPIRIT",
					fmt.Sprintf("task %s depends on non-existent task %s", id, depID))
			}
		}
	}
	return nil
}

func (d *TaskDAG) detectCycles() error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	colors := make(map[TaskNodeID]int, len(d.Nodes))
	for id := range d.Nodes {
		colors[id] = white
	}
	var dfs func(TaskNodeID) error
	dfs = func(id TaskNodeID) error {
		colors[id] = gray
		node := d.Nodes[id]
		for _, depID := range node.DependsOn {
			switch colors[depID] {
			case gray:
				return kerrors.BadRequest("SPIRIT",
					fmt.Sprintf("cycle detected: task %s → %s", id, depID))
			case white:
				if err := dfs(depID); err != nil {
					return err
				}
			}
		}
		colors[id] = black
		return nil
	}
	for id := range d.Nodes {
		if colors[id] == white {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *TaskDAG) computeRoots() []TaskNodeID {
	hasIncoming := make(map[TaskNodeID]bool, len(d.Nodes))
	for id := range d.Nodes {
		hasIncoming[id] = false
	}
	for _, node := range d.Nodes {
		for _, depID := range node.DependsOn {
			hasIncoming[depID] = true
		}
	}
	var roots []TaskNodeID
	for id, has := range hasIncoming {
		if !has {
			roots = append(roots, id)
		}
	}
	return roots
}

func (d *TaskDAG) ReadyNodes(completed map[TaskNodeID]bool) []*TaskNode {
	var ready []*TaskNode
	for id, node := range d.Nodes {
		if completed[id] {
			continue
		}
		allDepsDone := true
		for _, depID := range node.DependsOn {
			if !completed[depID] {
				allDepsDone = false
				break
			}
		}
		if allDepsDone {
			ready = append(ready, node)
		}
	}
	return ready
}

func (d *TaskDAG) IsComplete(completed map[TaskNodeID]bool) bool {
	for id := range d.Nodes {
		if !completed[id] {
			return false
		}
	}
	return true
}

type TopologyType string

const (
	TopologyParallel    TopologyType = "parallel"
	TopologySequential  TopologyType = "sequential"
	TopologyHybrid      TopologyType = "hybrid"
	TopologyCoordinator TopologyType = "coordinator"
)

func (d *TaskDAG) RouteTopology() TopologyType {
	if len(d.Nodes) == 0 {
		return TopologyCoordinator
	}
	if len(d.Roots) == len(d.Nodes) {
		return TopologyParallel
	}
	depth := d.computeDepth()
	width := d.computeMaxWidth()
	if depth > 3 {
		return TopologyCoordinator
	}
	if width > 1 {
		return TopologyHybrid
	}
	return TopologySequential
}

func (d *TaskDAG) calcDepthMap() map[TaskNodeID]int {
	depthMap := make(map[TaskNodeID]int, len(d.Nodes))
	var calcDepth func(id TaskNodeID) int
	calcDepth = func(id TaskNodeID) int {
		if dep, ok := depthMap[id]; ok {
			return dep
		}
		node := d.Nodes[id]
		if node == nil {
			return 0
		}
		maxDep := 0
		for _, depID := range node.DependsOn {
			if dep := calcDepth(depID); dep > maxDep {
				maxDep = dep
			}
		}
		depthMap[id] = maxDep + 1
		return depthMap[id]
	}
	for id := range d.Nodes {
		calcDepth(id)
	}
	return depthMap
}

func (d *TaskDAG) computeDepth() int {
	depthMap := d.calcDepthMap()
	maxDepth := 0
	for _, dep := range depthMap {
		if dep > maxDepth {
			maxDepth = dep
		}
	}
	return maxDepth
}

func (d *TaskDAG) computeMaxWidth() int {
	depthMap := d.calcDepthMap()
	levelMap := make(map[int]int)
	for _, level := range depthMap {
		levelMap[level]++
	}
	maxWidth := 0
	for _, w := range levelMap {
		if w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

func (d *TaskDAG) ToTextDiagram() string {
	if d == nil || len(d.Nodes) == 0 {
		return ""
	}
	depthMap := d.calcDepthMap()
	levelGroups := make(map[int][]*TaskNode)
	for id, depth := range depthMap {
		node := d.Nodes[id]
		if node != nil {
			levelGroups[depth] = append(levelGroups[depth], node)
		}
	}

	maxDepth := 0
	for _, depth := range depthMap {
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	var sb strings.Builder
	sb.WriteString("任务依赖图:\n")
	for level := 1; level <= maxDepth; level++ {
		nodes := levelGroups[level]
		if len(nodes) == 0 {
			continue
		}
		for i, node := range nodes {
			prefix := "  ├─ "
			if i == len(nodes)-1 {
				prefix = "  └─ "
			}
			name := node.TaskName
			if name == "" {
				name = string(node.ID)
			}
			sb.WriteString(prefix + name)
			if len(node.DependsOn) > 0 {
				depNames := make([]string, 0, len(node.DependsOn))
				for _, depID := range node.DependsOn {
					if depNode, ok := d.Nodes[depID]; ok {
						depName := depNode.TaskName
						if depName == "" {
							depName = string(depID)
						}
						depNames = append(depNames, depName)
					}
				}
				if len(depNames) > 0 {
					sb.WriteString(" ← " + strings.Join(depNames, ", "))
				}
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func InferTopologyFromTeam(team Team, lg loggateway.Logger) TopologyType {
	if team.Topology != "" {
		return TopologyType(team.Topology)
	}
	if len(team.DependsOn) > 0 {
		return TopologySequential
	}
	if team.ParallelConfigJSON != "" {
		cfg := ParseParallelConfig(team.ParallelConfigJSON, lg)
		if cfg.MaxConcurrentTeams > 1 {
			return TopologyParallel
		}
	}
	return TopologyCoordinator
}