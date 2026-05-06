// Package toolapi 定义本项目中「工具」的统一抽象与全局注册表：
//
//   - InvokeRequest / InvokeResponse：进程内调用的统一输入/输出信封（JSON 友好）。
//   - Tool 接口：同时具备本地可执行（SupportsLocalInvoke）、OpenAI 规格与 ADK binding。
//   - Default() 全局 Registry：由 registerstd/init 注册内置实现，供管理端 OpenAI 回路和 catalog 装配复用。
//
// 各具体工具请放在 internal/tools/<工具名>/ 子目录，单行职责 + 中文注释见各包 doc。
package toolapi
