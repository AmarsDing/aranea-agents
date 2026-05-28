---
name: "go-oop-guide"
description: "Go OOP 编程规范指导。当编写 Go 代码涉及 struct 设计、接口定义、抽象分层、组合嵌入、工厂构造时自动触发，提供面向对象最佳实践。"
---

# Go 面向对象编程规范

> **项目特定约束**（红线、分层、框架集成等）见 `aranea-coding-guide` SKILL。本文只提供通用 Go OOP 最佳实践，项目内编码以统一指南为准。

## 核心哲学

**组合优于继承，接口优于抽象类，小优于大。**

Go 不是传统 OOP 语言（无 class、继承、虚方法表），其抽象体系：

| 传统 OOP | Go 做法 |
|----------|---------|
| Class + 继承 | Struct + 组合（嵌入） |
| Interface 继承 | 接口组合（小接口拼大接口） |
| 多态靠虚表 | 隐式满足（duck typing） |
| 抽象基类 | 接口 + 默认实现函数 |
| 构造/析构 | 工厂函数 + `io.Closer` |

---

## 一、Struct + 方法 = 封装

### 规则

1. 字段小写 = 包内私有，大写 = 导出（包级可见性替代 private/protected/public）
2. 方法接收者：需要修改用 `*T`，只读或小结构体用 `T`
3. 构造用 `NewXxx()` 工厂函数，禁止裸 `&Xxx{}`
4. 构造函数参数只接收接口或具体依赖，不接收"上帝对象"
5. 依赖通过参数注入，禁止全局变量

### 示例

```go
type Order struct {
    id     string
    status OrderStatus
    items  []*OrderItem
}

func NewOrder(id string) *Order {
    return &Order{id: id, status: StatusOpen}
}

func (o *Order) AddItem(item *OrderItem) error {
    if o.status == StatusClosed {
        return kerrors.BadRequest("ORDER", "order is closed")
    }
    o.items = append(o.items, item)
    return nil
}
```

---

## 二、Embedding = 组合（非继承）

### 规则

1. 嵌入是**组合 + 方法提升**，不是 is-a 继承
2. 没有 `super` 调用，没有虚函数分派
3. 需要多态行为时，**必须配接口**，嵌入本身不提供多态
4. 嵌入用于共享通用字段/行为（如 `BaseEntity`），不用于建立类型层次

### 示例

```go
type BaseEntity struct {
    ID        string
    CreatedAt time.Time
    UpdatedAt time.Time
}

type Agent struct {
    BaseEntity
    Name     string
    Provider string
    Config   AgentConfig
}
```

---

## 三、Interface = 抽象 + 多态

### 黄金法则

> **接口要小，定义在使用方。**

### 规则

1. 接口方法不超过 5 个，超过则拆分
2. 接口定义在**使用方包**（如 `biz`），实现在**提供方包**（如 `data`）
3. 先写具体类型，需要抽象时再提取接口——不要先写接口再写实现
4. 函数**返回具体类型，参数接收接口**（Accept interfaces, return structs）
5. 禁止空接口 `interface{}` 滥用，用泛型或具体类型替代
6. 接口组合优于方法堆砌

### 好的接口

```go
type AgentRepository interface {
    Get(ctx context.Context, id string) (*Agent, error)
    Save(ctx context.Context, agent *Agent) error
}
```

### 差的接口

```go
type AgentRepository interface {
    Get(ctx context.Context, id string) (*Agent, error)
    Save(ctx context.Context, agent *Agent) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter Filter) ([]*Agent, error)
    Count(ctx context.Context, filter Filter) (int, error)
    BatchSave(ctx context.Context, agents []*Agent) error
}
```

### 接口组合

```go
type Reader interface { Read(p []byte) (n int, err error) }
type Writer interface { Write(p []byte) (n int, err error) }
type ReadWriter interface {
    Reader
    Writer
}
```

### 隐式满足

```go
type Notifier interface {
    Notify(ctx context.Context, event Event) error
}

type EmailNotifier struct{ smtp string }
func (n *EmailNotifier) Notify(ctx context.Context, event Event) error { ... }

type SlackNotifier struct{ webhook string }
func (n *SlackNotifier) Notify(ctx context.Context, event Event) error { ... }

func NewUsecase(notifier Notifier) *Usecase { ... }
```

---

## 四、端口-适配器（六边形架构）

### 规则

1. 接口定义在 biz 层（端口），实现在 data 层（适配器）
2. biz 层不得 import data 层或基础设施包
3. 跨模块调用通过 biz 级窄接口（端口），不持有对方 Service 完整类型
4. Wire 绑定在 Service 层

### 示例

```go
// biz/order.go — 端口
type OrderRepository interface {
    Get(ctx context.Context, id string) (*Order, error)
    Save(ctx context.Context, order *Order) error
}

type OrderUsecase struct {
    repo   OrderRepository
    events EventPublisher
}

// data/order.go — 适配器
type orderRepo struct {
    db *ent.Client
}

func (r *orderRepo) Get(ctx context.Context, id string) (*Order, error) {
    entOrder, err := r.db.Order.Get(ctx, id)
    return entOrderToBiz(entOrder), err
}
```

---

## 五、窄接口原则

```go
// ❌ 持有对方完整类型——耦合
type ChatUsecase struct {
    agentSvc *AgentService
}

// ✅ 窄接口——只暴露需要的
type AgentLookup interface {
    GetAgentConfig(ctx context.Context, id string) (*AgentConfig, error)
}

type ChatUsecase struct {
    agentLookup AgentLookup
}
```

---

## 六、Functional Options 模式

用于灵活构造，当结构体有多个可选配置时：

```go
type AgentOption func(*Agent)

func WithProvider(p string) AgentOption {
    return func(a *Agent) { a.Provider = p }
}

func WithModel(m string) AgentOption {
    return func(a *Agent) { a.Model = m }
}

func NewAgent(name string, opts ...AgentOption) *Agent {
    a := &Agent{Name: name}
    for _, opt := range opts {
        opt(a)
    }
    return a
}

agent := NewAgent("gpt4", WithProvider("openai"), WithModel("gpt-4"))
```

---

## 七、中间件/装饰器模式

用于横切关注点（日志、鉴权、限流、指标）：

```go
type Handler interface {
    Handle(ctx context.Context, req Request) (Response, error)
}

type LoggingMiddleware struct {
    next Handler
}

func (m *LoggingMiddleware) Handle(ctx context.Context, req Request) (Response, error) {
    start := time.Now()
    resp, err := m.next.Handle(ctx, req)
    log.Printf("duration=%v err=%v", time.Since(start), err)
    return resp, err
}
```

---

## 八、接口 + 默认实现

当需要可选覆盖行为时：

```go
type Validator interface {
    Validate() error
}

type DefaultValidator struct{}

func (DefaultValidator) Validate() error { return nil }

type Order struct {
    DefaultValidator
    ID string
}

func (o *Order) Validate() error {
    if o.ID == "" {
        return kerrors.BadRequest("ORDER", "id is required")
    }
    return nil
}
```

---

## 九、决策速查

```
需要多态？         → 接口
需要代码复用？     → 组合（嵌入 struct）
需要默认行为？     → 接口 + 默认实现函数
需要灵活构造？     → Functional Options
需要解耦模块？     → 端口-适配器（接口在 biz，实现在 data）
需要横切关注点？   → 中间件/装饰器
只在本包用？       → 不需要接口，直接用 struct
```

---

## 十、错误处理

1. 统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误
2. `fmt.Errorf` 仅用于 wrap 错误（`fmt.Errorf("get order: %w", err)`）
3. 错误变量用 `Err` 前缀（`ErrNotFound`）

```go
kerrors.BadRequest("AGENT", "id is required")
kerrors.NotFound("AGENT", "agent not found")
kerrors.InternalServer("AGENT", err.Error())
```

---

## 十一、并发

1. 所有跨层调用必须传递 `ctx`
2. goroutine 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover`
3. 共享状态用 `sync.Mutex` / `sync.RWMutex`，禁止全局变量

---

## 十二、命名约定

| 类别 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `agent`, `mcp/config` |
| 结构体/接口 | 大驼峰，名词 | `AgentUsecase`, `AgentRepository` |
| 函数 | 大驼峰导出/小驼峰内部 | `NewAgentUsecase`, `fromProtoRuntime` |
| 错误变量 | `Err` 前缀 | `ErrNotFound` |
| 接口 | 名词 + er 后缀（行为接口） | `Reader`, `Notifier` |
| 接口 | 名词（数据接口） | `AgentRepository`, `OrderStore` |
