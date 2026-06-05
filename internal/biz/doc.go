// Package biz implements the domain business logic layer (DDD use cases).
//
// This package follows the dependency rule: biz imports only data interfaces
// (repositories) and never imports pkg/trpc-agent-go directly.
//
// Sub-domains are organized into sub-packages:
//   - a2a/        — A2A proxy agent logic
//   - artifact/   — artifact attachments, limits, turn collection
//   - avatar/     — avatar image generation
//   - channelicons/ — embedded channel platform icons
//   - cron/       — scheduled job management
//   - ecosystem/  — ecosystem/marketplace logic
//   - evaluation/ — eval framework
//   - flowlog/    — flow log queries
//   - hook/       — webhook hook management
//   - knowledge/  — knowledge base CRUD
//   - monitor/    — audit logging, monitor events, alert evaluation
//   - plugin/     — plugin lifecycle
//   - session/    — session CRUD, timeline, turns, batch, export
//   - shared/     — shared domain helpers
//   - skill/      — skill management
//   - tool/       — tool catalog, policy, preview
//   - usage/      — usage tracking and quotas
//
// Top-level files by domain:
//   - Agent:     agent_usecase.go, agent_types.go, agent_*.go
//   - Channel:   channel.go, channel_*.go
//   - Chat:      chat_usecase.go, turn_*.go
//   - Graph:     graph_*.go, team_compiler.go
//   - Memory:    memory*.go, memory_l*.go
//   - Monitor:   monitor.go, runner_completion.go, audit_record.go
//   - Team:      team_usecase.go, team_*.go, orchestration_*.go
//   - Event Bus: event_bus_*.go, domain_event.go
//   - Evolution: evolution*.go
//   - Utility:   evaluation.go (Agent 评估模型与评分),
//               json_list.go (JSON 列表工具函数),
//               json_schema.go (JSON Schema 验证辅助),
//               pagination.go (通用分页模型)
package biz
