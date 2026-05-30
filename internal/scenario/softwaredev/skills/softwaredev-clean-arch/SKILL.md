# Clean Architecture 实践

## 分层
```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口
        ↓
internal/data           ← Repo 实现（Ent ORM）
```

## 依赖规则
- 跨层只允许向内依赖
- biz 层不得 import api proto 包
- biz 层不得 import pkg/trpc-agent-go
- Service 层不得写业务逻辑

## Usecase 模式
- 每个 Usecase 只依赖接口（Repo 接口定义在 biz 层）
- 构造函数参数只接收接口
- Wire 绑定在 Service 层
