package loader

// This file is intentionally empty.
//
// Historical "loader → biz direct write" paths (seedAgents / seedTeams /
// BuildBizAgentFromSpec / BuildBizTeamFromSpec / convertGraphSpec / resolveModel /
// jsonStringList / skillRuntimeJSON / resolveAgentKeys and the Deps struct)
// have been moved to their authoritative homes and then removed from this
// package. The current loader package is a pure YAML reader:
//
//   - LoadCompanySpec       (company_loader.go)      → CompanySpec
//   - LoadOrganizationSpec  (organization_loader.go) → OrganizationSpec
//   - LoadAgentTemplatesSpec(agent_templates_loader.go) → AgentTemplatesSpec
//
// Spec → Pack conversion lives in internal/data/pack_convert.go.
// Spec → biz direct write (deprecated) was the source of a layering violation
// (loader held a Deps struct pointing at *biz.AgentUsecase, breaking the
// "dependency direction inward" rule). It is gone and must not be reintroduced.
