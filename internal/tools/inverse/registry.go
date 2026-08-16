// Package inverse 维护「正工具 → 逆工具」声明注册表（P0-1，Cordis H3 补偿对
// 的业务层形态）。
//
// 声明是静态代码级元数据，随工具所在包的构造路径注册（如 twinops.NewToolset），
// 不侵入框架 Declaration。消费方是 internal/agent 的补偿跟踪器：
// 正工具调用成功后记 pending，逆工具调用成功后核销，超时未核销产生告警。
//
// 注册是幂等的：同工具同声明重复注册不产生变化（agent 每次构建都会重走
// 工具集构造路径，必须容忍重复 Register）。
package inverse

import "sync"

// Spec 描述一个工具的逆操作。
type Spec struct {
	// InverseTool 是撤销本工具副作用的工具名
	// （如 gns3_fault_inject → gns3_fault_clear）。
	InverseTool string
	// MapArgs 由正向调用参数推导逆操作参数（JSON in/out）。
	// nil 表示恒等：正/逆工具同 schema，参数直接复用。
	MapArgs func(args []byte) ([]byte, error)
}

var (
	mu      sync.RWMutex
	forward = map[string]Spec{}     // 正工具名 → 逆操作声明
	reverse = map[string][]string{} // 逆工具名 → 正工具名列表
)

// Register 注册正工具的逆操作声明。同工具同 InverseTool 重复注册为 no-op；
// 同工具换 InverseTool 视为更新（旧反向索引不清理——逆工具核销按名匹配，
// 旧索引残留仅多一次无效查找，无副作用）。
func Register(toolName string, spec Spec) {
	if toolName == "" || spec.InverseTool == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if old, ok := forward[toolName]; ok && old.InverseTool == spec.InverseTool {
		return
	}
	forward[toolName] = spec
	reverse[spec.InverseTool] = append(reverse[spec.InverseTool], toolName)
}

// LookupForward 返回正工具的逆操作声明。
func LookupForward(toolName string) (Spec, bool) {
	mu.RLock()
	defer mu.RUnlock()
	spec, ok := forward[toolName]
	return spec, ok
}

// IsInverse 报告 toolName 是否是某个已注册正工具的逆工具。
func IsInverse(toolName string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := reverse[toolName]
	return ok
}

// resetForTest 清空注册表，仅供本包测试使用。
func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	forward = map[string]Spec{}
	reverse = map[string][]string{}
}
