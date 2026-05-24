// Package adapter provides graph adapter implementations.
// Resolver implementations have been split into dedicated files:
//   - resolver_model.go  → CatalogModelResolver
//   - resolver_tool.go   → CatalogToolResolver
//   - resolver_agent.go  → CatalogAgentResolver
//
// This file re-exports the constructors for backward compatibility.
package adapter

// Resolver constructors are defined in their respective files.
// This file intentionally left minimal — all implementation code
// has been moved to resolver_*.go for single-responsibility clarity.
