// Package tools is the facade for callers outside this tree: ADK binding from biz effective tools.
//
// Layout:
//   - registry — canonical tool_key constants, workspace + ADK builtin wiring, legacy invoke helpers
//   - catalog — optional [catalog.Options] builder (skills, MCP, sub-agents)
//   - read_file, list_files, write_file, edit_file — workspace tool implementations
//   - shell_exec — host shell builtin
//   - workspace, argmap, specs — shared helpers for builtins
package tools
