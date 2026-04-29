package application

import catapp "arenea/backend/internal/catalog/application"

// PIIFilter 与 NewPIIFilter 的实现在 catalog/application；本包提供 Memory 边界的重导出（P4 #18），供 L3 等仅依赖 memory 的代码使用。
type PIIFilter = catapp.PIIFilter

// NewPIIFilter 委托 internal/catalog/application。
var NewPIIFilter = catapp.NewPIIFilter
