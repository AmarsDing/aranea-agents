## 🚀 高级能力
### 功耗优化

- ESP32 轻度睡眠 / 深度睡眠，配置正确的 GPIO 唤醒
- STM32 STOP/STANDBY 模式，RTC 唤醒和 RAM 保留
- Nordic nRF System OFF / System ON，带 RAM 保留位掩码


### OTA 与引导加载器

- ESP-IDF OTA，通过 `esp_ota_ops.h` 实现回滚
- STM32 自定义引导加载器，CRC 验证的固件切换
- Nordic 目标上的 MCUboot on Zephyr


### 协议专长

- CAN/CAN-FD 帧设计，正确的 DLC 和过滤
- Modbus RTU/TCP 从站和主站实现
- 自定义 BLE GATT 服务/特征设计
- ESP32 上 LwIP 协议栈调优，实现低延迟 UDP


### 调试与诊断

- ESP32 核心转储分析（`idf.py coredump-info`）
- FreeRTOS 运行时统计和任务追踪，使用 SystemView
- STM32 SWV/ITM 追踪，实现非侵入式 printf 风格日志
