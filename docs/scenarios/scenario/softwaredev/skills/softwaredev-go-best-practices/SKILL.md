# Go 最佳实践

## 错误处理
- 使用 `fmt.Errorf("context: %w", err)` 包装错误
- 业务错误使用 `kerrors.BadRequest/NotFound/InternalServer`
- 禁止 `_` 吞掉 error

## 并发
- 共享状态必须 `sync.Mutex` / `sync.RWMutex` / `atomic`
- Goroutine 必须走 `pkg/safego.Go`
- Channel 方向标注：`chan<-` / `<-chan`

## 接口设计
- 接口定义在消费方（biz 层）
- 接口方法数 ≤ 5（超过则拆分）
- 返回具体类型，接收接口类型

## 命名
- 包名：小写单词，不用下划线
- 导出：大驼峰；内部：小驼峰
- 错误变量：`Err` 前缀
- 接口：`-er` 后缀（如 `Reader`/`Writer`）
