## 📋 你的技术交付物
### 正确的边界设计（批量，不要频繁交互）

```rust
// wasm-bindgen — 错误的形状：每个元素一次调用意味着 N 次边界穿越
#[wasm_bindgen]
pub fn process_one(x: f64) -> f64 { x * x + 1.0 }   // 调用者在 JS 中循环 → 死于千次调用

// 正确的形状：将整个缓冲区交给模块，在 Wasm 内部循环，穿越一次
#[wasm_bindgen]
pub fn process_batch(input: &[f64], output: &mut [f64]) {
    for (i, &x) in input.iter().enumerate() {
        output[i] = x * x + 1.0;                    // 热循环保持原生速度，在模块内
    }
}
```

```javascript
// JS 侧：在 Wasm 线性内存的视图上操作——零逐元素复制
const inputPtr = wasm.alloc(n * 8);
const input = new Float64Array(wasm.memory.buffer, inputPtr, n);
input.set(sourceData);                 // 一次批量复制入
wasm.process_batch(inputPtr, n);       // 一次边界穿越
const result = new Float64Array(wasm.memory.buffer, outputPtr, n).slice(); // 一次批量复制出
// N 个元素 3 次边界交互，而非 N 次。这就是全部游戏。
```

### "这应该是 Wasm 吗？"决策表

| 工作负载 | Wasm 结论 | 原因 |
|----------|-------------|-----|
| 图像/视频/音频编解码器、压缩、加密 | ✅ 强力胜出 | 计算密集、紧凑循环、最小边界流量 |
| 物理、模拟、ML 推理内核 | ✅ 强力胜出 | 每次边界穿越大量数学；SIMD 友好 |
| 大缓冲区上的解析器/验证器 | ✅ 胜出 | 数据入一次，结果出一次 |
| DOM 操作、UI 胶水、事件处理 | ❌ 通常失败 | 每次触摸 DOM 都穿越边界；JS 已经在那里 |
| 频繁交互的逻辑含多次小 JS 交互 | ❌ 失败 | 编组成本压倒计算 |
| 不受信任的第三方插件（服务端或客户端）| ✅ 胜出（为安全）| 沙盒隔离是目的，即使性能持平 |
| 移植大型现有 C/C++/Rust 库 | ✅ 经常胜出 | 在浏览器中复用经过实战检验的原生代码 |

### 服务端 WASI + 能力沙盒（Wasmtime）

```rust
// 以恰好所需的能力运行不受信任的插件——无环境访问。
use wasmtime::*;
use wasmtime_wasi::WasiCtxBuilder;

let engine = Engine::new(Config::new().wasm_component_model(true))?;
let wasi = WasiCtxBuilder::new()
    .preopened_dir("./plugin-data", "/data",         // 仅此目录，映射读/写
        DirPerms::all(), FilePerms::all())?
    // 无网络、无环境变量、无其他文件系统——默认拒绝是安全模型
    .build();
// 插件实际上无法打开套接字或读取 /etc/passwd；宿主从未授予它。
```

### 二进制大小缩减管道

```bash
# 6MB 调试模块是加载时税。发布优化版本。
wasm-opt -Oz --strip-debug --dce input.wasm -o optimized.wasm   # 大小优先优化 + DCE
# Rust：release 配置中 opt-level="z"、lto=true、codegen-units=1、panic="abort"、strip=true
# 然后使用流式编译服务，使其在下载时编译：
#   WebAssembly.instantiateStreaming(fetch('optimized.wasm'), imports)
# 衡量：在 CI 中跟踪模块大小，像任何其他包预算一样——它会悄然增长。
```
