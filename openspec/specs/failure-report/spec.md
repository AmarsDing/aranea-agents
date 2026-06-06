## ADDED Requirements

### Requirement: FailureReport 标准化错误表示
系统 SHALL 定义 FailureReport 结构体，统一 CI 和运行时的错误描述格式。FailureReport MUST 包含以下字段：ID(UUID)、Type(lint_error/test_failure/build_failure/proto_sync/runtime_error)、Source(ci/runtime)、Job、File、Line、ErrorCode、Message、StackTrace、RelatedCode、Metadata。

#### Scenario: CI 日志解析为 FailureReport
- **WHEN** CI Auto-Fix 收到失败日志
- **THEN** 系统 SHALL 将原始日志解析为结构化 FailureReport JSON，包含 type/source/job/file/line/error_code/message 字段

#### Scenario: 运行时错误转换为 FailureReport
- **WHEN** 运行时自愈检测到错误
- **THEN** 系统 SHALL 将运行时错误信息转换为 FailureReport，source 字段为 "runtime"

### Requirement: FailureReport 解析器
系统 SHALL 提供 ParseCIlogs 和 ParseRuntimeError 函数，将原始错误信息转换为 FailureReport。

#### Scenario: 解析 Go 编译错误
- **WHEN** 输入 Go 编译错误日志
- **THEN** 解析器 SHALL 提取 file、line、error_code、message 字段，type 为 "build_failure"

#### Scenario: 解析 Go 测试失败
- **WHEN** 输入 Go 测试失败日志
- **THEN** 解析器 SHALL 提取 file、line、error_code、message、stack_trace 字段，type 为 "test_failure"

#### Scenario: 解析 Lint 错误
- **WHEN** 输入 lint 错误日志
- **THEN** 解析器 SHALL 提取 file、line、error_code、message 字段，type 为 "lint_error"
