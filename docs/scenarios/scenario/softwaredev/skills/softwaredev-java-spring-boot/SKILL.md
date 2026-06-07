# Skill: Java Spring Boot 最佳实践

## 概述
Java Spring Boot 后端开发的分层架构、依赖注入、异常处理、配置管理、事务管理、API 设计与安全规范。目标是构建可维护、可测试、高内聚低耦合的 Spring Boot 应用。

## 核心规则

### 1. 分层架构
- 严格四层分离：Controller → Service → Repository → Entity
- Controller 只做参数校验、调用 Service、返回响应，不含业务逻辑
- Service 承载业务逻辑，通过接口 + 实现分离便于测试和替换
- Repository 仅负责数据访问，使用 Spring Data JPA 或 MyBatis-Plus
- Entity 与数据库表映射，不携带业务行为时使用 DTO/VO 做视图转换
- 跨层依赖方向：Controller → Service → Repository，禁止反向依赖
- 层间数据传递使用 DTO，禁止直接暴露 Entity 到 API 响应

### 2. 依赖注入
- 优先使用构造函数注入，禁止 `@Autowired` 字段注入
- 构造函数注入配合 `lombok.RequiredArgsConstructor` 减少样板代码
- 必须注入的依赖声明为 `final`，可选依赖使用 `@Nullable`
- 避免循环依赖：通过事件驱动、中介者模式或接口拆分解耦
- Configuration 类使用 `@Bean` 方法注入，不使用 `@Autowired`

### 3. 异常处理
- 全局异常处理使用 `@ControllerAdvice` + `@ExceptionHandler`
- 自定义业务异常层级：`BaseException` → `BusinessException` / `ValidationException` / `AuthException`
- 异常携带错误码（枚举）+ 用户友好消息，禁止直接暴露堆栈
- 异常日志记录完整上下文（请求参数、用户 ID、traceId）
- 校验失败使用 `MethodArgumentNotValidException` 统一处理，返回字段级错误
- 禁止在 Service 层吞掉异常后返回 null，应抛出或包装后抛出

### 4. 配置管理
- 使用 `application.yml` 而非 `.properties`，支持层级结构
- 类型安全配置使用 `@ConfigurationProperties` + `@Validated`，禁止 `@Value` 批量取值
- 环境差异化配置：`application-{profile}.yml`，敏感信息走环境变量或 Vault
- 配置类使用 `@Configuration` + `@EnableConfigurationProperties`，不使用 `@Component`
- 默认值在配置类中声明，不依赖配置文件必须提供

### 5. 事务管理
- `@Transactional` 只标注在 Service 层公开方法上
- 只读操作使用 `@Transactional(readOnly = true)` 优化性能
- 事务方法必须是 `public`，非 public 方法 `@Transactional` 不生效
- 避免在事务方法内调用远程服务或发送消息（事务超时风险）
- 传播行为默认 `REQUIRED`，需要新事务时显式声明 `REQUIRES_NEW`
- 大批量操作拆分为小批次 + 编程式事务（`TransactionTemplate`），避免长事务

### 6. API 设计
- RESTful 风格：资源名用复数名词，动作用 HTTP 方法（GET/POST/PUT/DELETE）
- 统一响应体：`ApiResponse<T>` 包含 code/message/data，禁止裸返回值
- 分页参数标准化：`page`/`size`/`sort`，响应包含 `totalElements`/`totalPages`
- API 版本化：URL 路径版本 `/api/v1/` 或 Header 版本 `Accept: application/vnd.api.v1+json`
- 请求参数校验使用 `jakarta.validation` 注解（`@NotNull`/`@Size`/`@Pattern`）
- 幂等设计：写操作使用幂等键（`Idempotency-Key` Header）

### 7. 安全
- Spring Security 配置集中管理，禁止散装 `@PermitAll`
- JWT 无状态认证：Access Token 短期 + Refresh Token 长期
- RBAC 权限模型：角色 → 权限，方法级鉴权使用 `@PreAuthorize("hasAuthority('xxx')")`
- 密码存储使用 BCrypt（`PasswordEncoder`），禁止明文或 MD5
- 输入校验：SQL 注入防护（参数化查询）、XSS 防护（输出编码）、CSRF 防护
- 敏感端点限流：使用 `Bucket4j` 或 Redis 令牌桶

## 反模式（禁止）

- God Class：单个类超过 500 行或承担超过 2 个职责
- 贫血模型：Entity 只有 getter/setter 无行为，业务逻辑全在 Service 形成事务脚本
- N+1 查询：循环内调用 Repository 查询，应使用 JOIN FETCH 或批量查询
- `@Autowired` 字段注入：隐藏依赖关系，无法声明不可变依赖，妨碍测试
- 在 Controller 写业务逻辑：Controller 变成"胖 Controller"
- 事务内调用远程服务：网络超时导致数据库连接长时间占用
- 直接返回 Entity 到 API 响应：暴露内部数据结构，无法独立演进
- 使用 `@Value` 管理大量配置：无类型安全、无分组、无校验
