package biz

import "aranea-agents/internal/biz/ecosystem"

// Re-export ecosystem types from sub-package for backward compatibility.
type (
	EcosystemProduct       = ecosystem.Product
	EcosystemInstallResult = ecosystem.InstallResult
	EcosystemQuery         = ecosystem.Query
	EcosystemListResult    = ecosystem.ListResult
	EcosystemRepo          = ecosystem.Repo
	EcosystemUsecase       = ecosystem.Usecase
)

// Re-export ecosystem constructor for backward compatibility.
var NewEcosystemUsecase = ecosystem.NewUsecase
