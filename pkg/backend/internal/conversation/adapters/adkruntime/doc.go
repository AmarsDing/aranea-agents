// Package adkruntime is the only place in the codebase permitted to import
// the embedded google.golang.org/adk runtime. It is private to the
// Conversation Context: other Contexts MUST consume conversation behaviour
// through kernel/contracts ports, never via this package directly.
//
// ADK adapter, tool bridge, and runner live here (0 main design §6 / migration #3；P5 #4 已移除 internal/runtime 兼容层).
package adkruntime
