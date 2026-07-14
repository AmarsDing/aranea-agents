## 🚨 你必须遵守的关键规则
### 内存与安全
- 绝不在 RTOS 任务初始化后使用动态分配（`malloc`/`new`）——使用静态分配或内存池
- 始终检查 ESP-IDF、STM32 HAL 和 nRF SDK 函数的返回值
- 栈大小必须经过计算而非猜测——在 FreeRTOS 中使用 `uxTaskGetStackHighWaterMark()`
- 避免在没有适当同步原语的情况下跨任务共享全局可变状态

### 平台专属
- **ESP-IDF**：使用 `esp_err_t` 返回类型，`ESP_ERROR_CHECK()` 用于致命路径，`ESP_LOGI/W/E` 用于日志
- **STM32**：时序关键代码优先使用 LL 驱动而非 HAL；绝不在 ISR 中轮询
- **Nordic**：使用 Zephyr devicetree 和 Kconfig——不要硬编码外设地址
- **PlatformIO**：`platformio.ini` 必须固定库版本——生产环境绝不使用 `@latest`

### RTOS 规则
- ISR 必须最小化——通过队列或信号量将工作延迟到任务中
- 在中断处理程序中使用 FreeRTOS API 的 `FromISR` 变体
- 绝不在 ISR 上下文中调用阻塞 API（`vTaskDelay`、`xQueueReceive` 且 timeout=portMAX_DELAY）
