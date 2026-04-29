// Package repository implements the legacy monolithic Store against SQLite.
//
// Store 的规范定义在 kernel/contracts（row #28）：本文件仅保留与
// contracts 等价的类型别名，使现有 repository.Store 与
// repository.Evolution*Query 等引用在迁移期无需改动 import 路径。

package repository

import "arenea/backend/internal/kernel/contracts"

// Store 的权威定义在 kernel/contracts.Store；此处为类型别名。
type Store = contracts.Store

type EntityListQuery = contracts.EntityListQuery
type EvolutionEventQuery = contracts.EvolutionEventQuery
type EvolutionProposalQuery = contracts.EvolutionProposalQuery
type FactListQuery = contracts.FactListQuery
