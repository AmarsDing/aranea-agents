# Skill: Python 后端最佳实践

## 概述
Python 后端开发的项目结构、类型注解、异步编程、错误处理、数据验证、测试与性能规范。目标是构建类型安全、高性能、可测试的 Python 后端服务。

## 核心规则

### 1. 项目结构
- 使用 src layout：`src/<package>/` 作为主代码目录，`tests/` 在项目根目录
- `pyproject.toml` 作为唯一项目配置入口（依赖、构建、工具配置）
- 依赖管理使用 `uv` 或 `poetry`，禁止 `requirements.txt` 手动维护
- 模块按领域划分：`src/<package>/users/`、`src/<package>/orders/`，禁止按技术层划分
- 每个领域模块包含：`router.py`（路由）、`service.py`（业务）、`repo.py`（数据访问）、`schema.py`（DTO）
- 共享代码放 `src/<package>/common/`：异常、中间件、工具函数

### 2. 类型注解
- 启用 `mypy --strict`，所有公开函数必须有完整类型签名
- 使用 `TypeAlias` 定义类型别名：`UserId = NewType("UserId", int)`
- 使用 `Protocol` 定义接口而非 `ABC`，支持结构化子类型
- 返回值使用 `TypedDict` 或 Pydantic Model，禁止返回裸 `dict`
- 可选值显式标注 `| None`，禁止隐式 Optional
- 集合类型标注元素类型：`list[User]` 而非 `list`
- 使用 `typing.override` 标注重写方法，`typing.final` 标注不可重写

### 3. 异步编程
- IO 密集型服务必须使用 `asyncio` + 异步框架（FastAPI / aiohttp）
- 数据库访问使用异步驱动（`asyncpg` / `aiomysql`），禁止在异步上下文中调用同步驱动
- HTTP 客户端使用 `httpx.AsyncClient`，配合连接池复用
- 异步任务使用 `asyncio.TaskGroup`（Python 3.11+），避免手动 `asyncio.gather`
- 长时间运行任务使用后台任务队列（Celery / ARQ），不在请求处理中阻塞
- 禁止在异步函数中使用 `time.sleep()`，使用 `asyncio.sleep()`
- 异步上下文管理器确保资源释放：`async with` 而非手动 `acquire/release`

### 4. 错误处理
- 自定义异常层级：`AppError` → `NotFoundError` / `ValidationError` / `AuthError` / `ConflictError`
- 每个异常携带机器可读错误码 + 用户消息 + HTTP 状态码
- FastAPI 使用 `@app.exception_handler` 统一异常到结构化 JSON 响应
- 结构化错误响应格式：`{"error": {"code": "USER_NOT_FOUND", "message": "...", "details": [...]}}`
- 禁止裸 `except:` 或 `except Exception:` 吞掉异常，至少记录日志后 re-raise
- 使用 `raise ... from err` 保留异常链，禁止 `raise NewError()` 丢失原始堆栈
- 校验错误返回字段级详情，而非笼统的 "参数错误"

### 5. 数据验证
- 使用 Pydantic v2 Model 做请求/响应验证，禁止手动 `if field is None` 校验
- 输入 Model 与输出 Model 分离：`UserCreate` / `UserUpdate` / `UserResponse`
- 复杂跨字段校验使用 `@model_validator`，而非 `@field_validator` 拼接
- 配置类使用 `model_config = ConfigDict(...)` 替代 v1 的 `Config` 内部类
- 序列化控制使用 `@computed_field` 和 `model_dump(exclude=...)`，禁止在 Model 外手动构造 dict
- 数据库 ORM Model 与 API Schema 严格分离，通过映射函数转换

### 6. 测试
- 使用 `pytest` + `pytest-asyncio` + `pytest-cov`
- 测试分层：单元测试（`tests/unit/`）、集成测试（`tests/integration/`）、E2E（`tests/e2e/`）
- Fixtures 按作用域组织：`conftest.py` 层级化，`scope="session"` 共享昂贵资源
- 异步测试标记 `@pytest.mark.asyncio`，禁止在同步测试中 `asyncio.run()`
- 参数化测试使用 `@pytest.mark.parametrize`，覆盖边界值和异常路径
- 测试隔离：每个测试独立数据库事务，测试结束回滚
- 覆盖率目标：核心业务逻辑 ≥ 80%，禁止 `# pragma: no cover` 掩盖问题

### 7. 性能
- 数据库查询使用连接池，池大小按 `CPU核心数 * 2 + 磁盘数` 估算
- 热点数据使用 Redis 缓存，缓存键带版本号或 TTL，禁止永不过期
- 批量操作使用批量插入/更新，禁止循环单条操作
- CPU 密集型任务卸载到进程池（`ProcessPoolExecutor`），不阻塞事件循环
- 性能分析使用 `py-spy` 或 `cProfile`，基于数据优化而非猜测
- 响应压缩启用 `gzip` 中间件，大列表响应支持流式分页

## 反模式（禁止）

- 全局可变状态：模块级 `cache = {}` 或全局变量，应使用依赖注入或缓存服务
- 裸 `except:` 或 `except Exception: pass`：吞掉所有异常且无日志
- 同步阻塞 IO 在异步上下文中：`requests.get()` 阻塞事件循环
- 未类型化的 `dict` 作为返回值：无法静态检查，应使用 TypedDict 或 Pydantic Model
- 在业务逻辑中直接操作 ORM Model 返回给 API：内部结构泄露
- 循环内单条数据库查询：N+1 问题，应使用批量查询或 JOIN
- 手动维护 `requirements.txt`：依赖版本漂移，应使用锁文件
- `asyncio.create_task` 不持有引用：任务被 GC 回收导致静默失败
