package twinops

import "aranea-agents/internal/tools/inverse"

// registerCompensationPairs 注册 twinops 工具的补偿对声明（P0-1，幂等）。
// gns3_fault_inject 的副作用由 gns3_fault_clear 撤销；两工具同 schema
// （gns3PortInput），参数恒等映射，无需 MapArgs。
func registerCompensationPairs() {
	inverse.Register("gns3_fault_inject", inverse.Spec{InverseTool: "gns3_fault_clear"})
}
