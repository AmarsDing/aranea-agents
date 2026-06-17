---
name: "go-oop-review"
description: "Go OOP 代码审查指导。当审查 Go 代码的 struct 设计、接口抽象、组合模式、依赖注入、分层架构时自动触发，提供结构化审查清单。"
---

# Go OOP 代码审查指导

> **项目特定审查规则**（红线、分层合规、框架集成等）见 `aranea-coding-guide` SKILL 第十二章自检清单。本文只提供通用 Go OOP 审查清单，项目内审查以统一指南为准。

## 审查流程

1. **架构层审查** → 检查分层与依赖方向
2. **接口审查** → 检查接口设计与抽象合理性
3. **Struct 审查** → 检查封装与组合
4. **并发安全审查** → 检查 goroutine 与共享状态
5. **错误处理审查** → 检查错误传播与业务语义

---

## 一、架构层审查

### 检查项

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| A1 | 依赖方向是否向内 | 🔴 阻断 | `biz` 不得 import `data`/`service`/基础设施包 |
| A2 | 接口是否定义在使用方 | 🟡 建议 | 端口接口应在 `biz`，实现在 `data` |
| A3 | Runner 装配是否在 Service 层 | 🔴 阻断 | `server` 层不得 new `runner.Runner` |
| A4 | Service 层是否有业务逻辑 | 🟡 建议 | Service 只做 proto↔biz 映射 + Runner 编排 |
| A5 | 跨模块是否通过窄接口 | 🟡 建议 | 不持有对方 Service 完整具体类型 |
| A6 | Wire 绑定是否在 Service 层 | 🟡 建议 | biz 层只定义接口，Wire 绑定收口在 Service |

### 审查模板

```
[架构层]
- 依赖方向：✅/❌ (说明违规处)
- 接口归属：✅/❌ (哪个接口放错了位置)
- Runner 装配：✅/❌
- Service 职责：✅/❌
- 跨模块调用：✅/❌
```

---

## 二、接口审查

### 检查项

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| I1 | 接口方法数 ≤ 5 | 🟡 建议 | 超过 5 个方法应拆分为多个小接口 |
| I2 | 接口是否定义在使用方 | 🟡 建议 | 使用方包定义接口，提供方包实现 |
| I3 | 是否存在"上帝接口" | 🔴 阻断 | 合并了多个不相关职责的接口必须拆分 |
| I4 | 返回值是否为接口类型 | 🟡 建议 | 函数应返回具体类型，参数接收接口 |
| I5 | 是否滥用 `interface{}` | 🔴 阻断 | 用泛型或具体类型替代 |
| I6 | 是否先写接口再写实现 | 🟢 提示 | 应先写具体类型，需要抽象时再提取接口 |
| I7 | 接口是否可组合 | 🟢 提示 | 大接口应拆为小接口 + 组合 |

### 常见反模式

#### 反模式 1：上帝接口

```go
// ❌ 审查不通过
type Repository interface {
    Get() error
    Save() error
    Delete() error
    List() error
    Count() error
    BatchSave() error
    Export() error
    Import() error
}

// ✅ 审查通过：拆分为小接口
type Reader interface {
    Get(ctx context.Context, id string) (*Entity, error)
    List(ctx context.Context, filter Filter) ([]*Entity, error)
}

type Writer interface {
    Save(ctx context.Context, entity *Entity) error
    Delete(ctx context.Context, id string) error
}

type Repository interface {
    Reader
    Writer
}
```

#### 反模式 2：返回接口

```go
// ❌ 审查不通过
func NewOrderRepo(db *ent.Client) OrderRepository { ... }

// ✅ 审查通过：返回具体类型
func NewOrderRepo(db *ent.Client) *orderRepo { ... }
```

#### 反模式 3：接口定义在实现方

```go
// ❌ 审查不通过：接口在 data 包
package data
type OrderRepository interface { ... }

// ✅ 审查通过：接口在 biz 包（使用方）
package biz
type OrderRepository interface { ... }
```

---

## 三、Struct 审查

### 检查项

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| S1 | 是否用工厂函数构造 | 🟡 建议 | 禁止裸 `&Xxx{}`，用 `NewXxx()` |
| S2 | 嵌入是否用于组合而非继承 | 🟡 建议 | 嵌入不建立 is-a 关系，多态靠接口 |
| S3 | 方法接收者是否一致 | 🟡 建议 | 同一类型所有方法统一用 `*T` 或 `T` |
| S4 | 构造函数是否接收上帝对象 | 🔴 阻断 | 只接收接口或具体依赖 |
| S5 | 是否暴露了不应导出的字段 | 🟡 建议 | 小写字段保持包内私有 |
| S6 | 是否有循环嵌入 | 🔴 阻断 | 嵌入不得形成循环 |

### 常见反模式

#### 反模式 1：裸构造

```go
// ❌ 审查不通过
agent := &Agent{Name: "gpt4"}

// ✅ 审查通过
agent := NewAgent("gpt4", WithProvider("openai"))
```

#### 反模式 2：继承思维嵌入

```go
// ❌ 审查不通过：试图用嵌入实现多态
type BaseHandler struct{}
func (b *BaseHandler) Handle() { /* default */ }

type AuthHandler struct { BaseHandler }
// 期望 BaseHandler.Handle() 能分派到 AuthHandler 的重写——Go 不会

// ✅ 审查通过：用接口实现多态
type Handler interface { Handle(ctx context.Context, req Request) (Response, error) }

type BaseHandler struct{}
func (b *BaseHandler) Handle(ctx context.Context, req Request) (Response, error) { ... }
```

#### 反模式 3：上帝对象注入

```go
// ❌ 审查不通过
func NewUsecase(app *App) *Usecase { ... }

// ✅ 审查通过：窄依赖
func NewUsecase(repo OrderRepository, notifier Notifier) *Usecase { ... }
```

---

## 四、并发安全审查

### 检查项

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| C1 | `go func()` 是否走 safego | 🔴 阻断 | 必须用 `pkg/safego.Go` / `pkg/safego.GoRecover` |
| C2 | 跨层调用是否传递 ctx | 🟡 建议 | 所有跨层调用必须传递 `ctx` |
| C3 | 共享状态是否有锁保护 | 🔴 阻断 | 共享状态用 `sync.Mutex`/`sync.RWMutex` |
| C4 | 是否有竞态条件 | 🔴 阻断 | map 并发读写、slice 并发 append |
| C5 | goroutine 是否可取消 | 🟡 建议 | 长期运行的 goroutine 应支持 ctx 取消 |

---

## 五、错误处理审查

### 检查项

| # | 检查项 | 严重级别 | 判定标准 |
|---|--------|----------|----------|
| E1 | 业务错误是否用 apierror | 🔴 阻断 | 禁止 `fmt.Errorf` 返回业务错误 |
| E2 | 错误是否丢失上下文 | 🟡 建议 | wrap 错误用 `fmt.Errorf("xxx: %w", err)` |
| E3 | 错误变量是否 Err 前缀 | 🟢 提示 | `ErrNotFound` 而非 `NotFoundError` |
| E4 | 是否吞掉了错误 | 🔴 阻断 | `_ = someFunc()` 忽略 error 返回值 |
| E5 | panic 是否被 recover | 🔴 阻断 | 业务代码禁止裸 panic |

---

## 六、审查输出模板

```markdown
## Go OOP 代码审查报告

### 概要
- 文件/模块：
- 审查范围：
- 发现问题数：🔴 X  🔴  🟡 Y  🟢 Z

### 🔴 阻断问题（必须修复）

| # | 位置 | 类别 | 描述 | 修复建议 |
|---|------|------|------|----------|
| 1 | file:line | 架构层 | biz import 了 data 包 | 移除直接依赖，通过接口解耦 |
| 2 | file:line | 接口 | 上帝接口 15 个方法 | 拆分为 Reader + Writer + ... |

### 🟡 建议改进

| # | 位置 | 类别 | 描述 | 改进建议 |
|---|------|------|------|----------|
| 1 | file:line | Struct | 裸 &Xxx{} 构造 | 改用 NewXxx() 工厂函数 |
| 2 | file:line | 接口 | 接口定义在 data 包 | 移到 biz 包定义端口 |

### 🟢 提示

| # | 位置 | 类别 | 描述 |
|---|------|------|------|
| 1 | file:line | 接口 | 可考虑先写实现再提取接口 |

### 架构合规性

- [ ] 依赖方向向内
- [ ] 接口在使用方定义
- [ ] Runner 装配在 Service 层
- [ ] Service 层无业务逻辑
- [ ] 跨模块通过窄接口
- [ ] Wire 绑定在 Service 层

### 接口合规性

- [ ] 接口方法 ≤ 5
- [ ] 无上帝接口
- [ ] 返回具体类型，参数接收接口
- [ ] 无 interface{} 滥用
- [ ] 接口可组合

### Struct 合规性

- [ ] 使用工厂函数构造
- [ ] 嵌入用于组合非继承
- [ ] 方法接收者一致
- [ ] 无上帝对象注入

### 并发安全

- [ ] goroutine 走 safego
- [ ] 跨层传递 ctx
- [ ] 共享状态有锁保护
- [ ] 无竞态条件

### 错误处理

- [ ] 业务错误用 apierror
- [ ] 错误 wrap 保留上下文
- [ ] 无吞错误
- [ ] 无裸 panic
```

---

## 七、审查决策树

```
发现接口 → 方法数 > 5？ → 是 → 🔴 拆分
                  → 否 → 定义在使用方？ → 否 → 🟡 移到使用方
                                      → 是 → ✅

发现 struct → 裸构造？ → 是 → 🟡 改工厂函数
           → 嵌入？ → 继承思维？ → 是 → 🟡 改用接口多态
                              → 否 → ✅
           → 上帝对象注入？ → 是 → 🔴 改窄依赖

发现 go func() → 走 safego？ → 否 → 🔴 必须改
              → 传 ctx？ → 否 → 🟡 加 ctx

发现 error → fmt.Errorf 业务错误？ → 是 → 🔴 改 apierror
          → 吞错误？ → 是 → 🔴 必须处理
          → wrap 丢上下文？ → 是 → 🟡 加 %w

发现 import → biz import data？ → 是 → 🔴 阻断
            → biz import pkg/trpc-agent-go？ → 是 → 🔴 阻断
            → biz import api proto？ → 是 → 🔴 阻断
```

---

## 八、严重级别定义

| 级别 | 含义 | 处理要求 |
|------|------|----------|
| 🔴 阻断 | 违反架构铁律，会导致耦合/安全问题 | 必须修复后才能合并 |
| 🟡 建议 | 不符合最佳实践，影响可维护性 | 建议修复，可延后 |
| 🟢 提示 | 风格偏好或微优化 | 可选改进 |
