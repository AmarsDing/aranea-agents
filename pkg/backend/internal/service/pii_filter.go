package service

import memapp "arenea/backend/internal/memory/application"

// PIIFilter 在 Memory 应用层对 Catalog 的 PIIFilter 重导；本包仅作类型别名以兼容原 import 路径。
type PIIFilter = memapp.PIIFilter

var NewPIIFilter = memapp.NewPIIFilter
