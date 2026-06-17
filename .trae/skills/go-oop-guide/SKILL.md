---
name: "go-oop-guide"
description: "Go OOP 编程规范指导。当编写 Go 代码涉及 struct 设计、接口定义、抽象分层、组合嵌入、工厂构造时自动触发，提供面向对象最佳实践。"
---

# Go 面向对象编程规范

> **项目特定约束**（红线、分层、框架集成、错误处理、并发、命名等）见 `aranea-coding-guide` SKILL。本文只提供**通用 Go OOP 最佳实践**，不含项目特定约束。

## 核心哲学

**组合优于继承，接口优于抽象类，小优于大。**

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
        return fmt.Errorf("order is closed")
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

type Product struct {
    BaseEntity
    Name  string
    Price float64
    Stock int
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
type OrderRepository interface {
    Get(ctx context.Context, id string) (*Order, error)
    Save(ctx context.Context, order *Order) error
}
```

### 差的接口

```go
type OrderRepository interface {
    Get(ctx context.Context, id string) (*Order, error)
    Save(ctx context.Context, order *Order) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter Filter) ([]*Order, error)
    Count(ctx context.Context, filter Filter) (int, error)
    BatchSave(ctx context.Context, orders []*Order) error
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

1. 接口定义在领域层（端口），实现在基础设施层（适配器）
2. 领域层不得 import 基础设施层或传输层包
3. 跨模块调用通过领域级窄接口（端口），不持有对方完整类型
4. 依赖注入在组合层绑定

### 示例

```go
// domain/order.go — 端口
type OrderRepository interface {
    Get(ctx context.Context, id string) (*Order, error)
    Save(ctx context.Context, order *Order) error
}

type OrderUsecase struct {
    repo   OrderRepository
    events EventPublisher
}

// infra/order.go — 适配器
type orderRepo struct {
    db *ent.Client
}

func (r *orderRepo) Get(ctx context.Context, id string) (*Order, error) {
    entOrder, err := r.db.Order.Get(ctx, id)
    return entOrderToDomain(entOrder), err
}
```

---

## 五、窄接口原则

```go
// ❌ 持有对方完整类型——耦合
type OrderUsecase struct {
    userSvc *UserService
}

// ✅ 窄接口——只暴露需要的
type UserLookup interface {
    GetUserEmail(ctx context.Context, id string) (string, error)
}

type OrderUsecase struct {
    userLookup UserLookup
}
```

---

## 六、Functional Options 模式

用于灵活构造，当结构体有多个可选配置时：

```go
type ServerOption func(*Server)

func WithHost(h string) ServerOption {
    return func(s *Server) { s.Host = h }
}

func WithPort(p int) ServerOption {
    return func(s *Server) { s.Port = p }
}

func NewServer(name string, opts ...ServerOption) *Server {
    s := &Server{Name: name}
    for _, opt := range opts {
        opt(s)
    }
    return s
}

server := NewServer("api", WithHost("0.0.0.0"), WithPort(8080))
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
        return fmt.Errorf("id is required")
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
需要解耦模块？     → 端口-适配器（接口在领域层，实现在基础设施层）
需要横切关注点？   → 中间件/装饰器
只在本包用？       → 不需要接口，直接用 struct
```

---

## 十、命名约定

| 类别 | 规则 | 示例 |
|------|------|------|
| 包名 | 小写单词，不用下划线 | `order`, `user/config` |
| 结构体/接口 | 大驼峰，名词 | `OrderUsecase`, `OrderRepository` |
| 函数 | 大驼峰导出/小驼峰内部 | `NewOrderUsecase`, `fromResponse` |
| 错误变量 | `Err` 前缀 | `ErrNotFound` |
| 接口 | 名词 + er 后缀（行为接口） | `Reader`, `Notifier` |
| 接口 | 名词（数据接口） | `OrderRepository`, `UserStore` |

---

## 项目特定约束引用

以下内容不在本文范围，见 `aranea-coding-guide` SKILL：

| 内容 | 位置 |
|------|------|
| 错误处理（apierror） | `aranea-coding-guide` §7.1 |
| 并发（safego、ctx 传递） | `aranea-coding-guide` §7.3 |
| 日志（loggateway.Logger） | `aranea-coding-guide` §7.4 |
| 依赖注入（Wire） | `aranea-coding-guide` §7.2 |
| 项目命名补充 | `aranea-coding-guide` §7.5 |
