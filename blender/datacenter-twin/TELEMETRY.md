# TELEMETRY.md — 数据中心数字孪生统一遥测契约

> 适用资产：`blender-switch-models`（交换机/UPS）、`server-3d`（服务器）、`blender-rack`（机柜）。
> 本文是 UE5 数据绑定的**唯一真相源**：设备 ID 规范、各设备 schema、字段 → 3D 对象/材质的绑定映射。
> 装配关系见 [datacenter_layout.json](./datacenter_layout.json)。

---

## 1. 设备 ID 规范

```
<rack_id>.<device_key>            设备实例 ID，如 A01.srv1 / A02.sw3 / UPS-A
<device_id>.<metric_path>         遥测点位 ID，如 A01.srv1.nics[0].activity
```

- `rack_id` / `device_key` 来自 `datacenter_layout.json` 的 `racks[].rack_id` 与 `units[].device_id`
- 塔式设备（floor_items）直接用 `item_id` 作为设备 ID，如 `UPS-A.mode`
- 机柜级环境点位挂在机柜 ID 下：`A01.env.temp_inlet_c`、`A01.pdu.left.power_w`
- 所有 ID 全小写；数组下标从 0 开始

## 2. 通用字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备实例 ID（§1） |
| `ts` | int64 | 采集时间戳（Unix 秒） |
| 枚举状态 | string | 各 schema 内标注的有限枚举，禁止自由文本 |

推送频率建议：状态/告警类 ≤1s；能耗/温度类 5–15s；静态信息（型号/序列号）仅变更时。

---

## 3. 服务器 schema（`SRV_1U/2U/4U`）

数据绑定路径前缀 `server.*`，FBX 内每个 LED 对象的 custom prop `data_bind` 已按下表写入。

```json
{
 "device": "A01.srv1", "ts": 1786200000,
 "power": "on | off | standby",
 "health": "ok | warning | critical",
 "alert": "none | fault",
 "uid": "on | off",
 "nics": [{"activity": "link | activity | down"}],
 "disks": [{"activity": "activity | fault | off", "temp_c": 38.0}],
 "psus": [{"status": "ok | fail | absent", "power_w": 320}],
 "cpus": [{"temp_c": 62.0, "util_pct": 45}],
 "gpus": [{"temp_c": 71.0, "util_pct": 88, "mem_used_gb": 60.2, "power_w": 340}],
 "fans": [{"rpm": 6800}],
 "mem": {"used_gb": 210.5, "total_gb": 512}
}
```

### 绑定映射（对象 custom props → 字段）

| 对象/材质 | custom prop | 字段 | 状态 → 表现 |
|-----------|-------------|------|-------------|
| `M_LED_Power` | `data_bind=server.power` | `power` | on=绿 / off=灭 / standby=琥珀 |
| `M_LED_Health` | `server.health` | `health` | ok=绿 / warning=琥珀 / critical=红 |
| `M_LED_Alert` | `server.alert` | `alert` | none=灭 / fault=琥珀闪 |
| `M_LED_UID` | `server.uid` | `uid` | on=蓝 / off=灭 |
| `M_LED_NIC`（每口） | `server.nics[i].activity` | `nics[i].activity` | link=绿 / activity=绿闪 / down=灭 |
| `M_LED_Disk`（每盘） | `server.disks[i].activity` | `disks[i].activity` | activity=绿闪 / fault=琥珀 / off=灭 |
| PSU LED（每 PSU） | `server.psus[i].status` | `psus[i].status` | ok=绿 / fail=红 / absent=灭 |

> 完整对象清单与 `led_type`/`led_desc`：`server-3d/ue5_integration.json`。
> 端口/UI 挂点坐标：同文件 `assembly.<tag>.ports[].pos_local`（设备局部坐标）。

## 4. 交换机 schema（`SW1/SW2/SW3`）

```json
{
 "device": "A01.sw1", "ts": 1786200000,
 "sys": {"pwr": "ok | fail", "sys": "ok | warning | critical", "stk": "master | slave | none"},
 "ports": [{"index": 1, "ifname": "GigabitEthernet0/0/1",
            "link": "up | down", "activity": true,
            "rx_mbps": 512.4, "tx_mbps": 128.0, "alarm": "none | alarm"}],
 "temp_c": 41.0, "cpu_pct": 23, "mem_pct": 38
}
```

### 绑定映射

| 对象 | 字段 | 状态 → 表现 |
|------|------|-------------|
| `{PREFIX}_LED_P{nn}_LNK` | `ports[i].link` | up=绿常亮 / down=灭（`M_LED_OFF`） |
| `{PREFIX}_LED_P{nn}_ACT` | `ports[i].activity` | true=琥珀闪 / false=灭 |
| `{PREFIX}_LED_SYS_{PWR\|SYS\|STK}` | `sys.*` | 见各 manifest `led_semantics` |
| `{PREFIX}_ANCHOR_P{nn}`（Empty） | 端口 UI 挂点 | 悬浮标签/Tooltip 定位 |

> 端口清单（含 `pos_mm`、`ifname`、`speed_mbps`、`poe`、`vlan`）：`blender-switch-models/models/{PREFIX}_manifest.json`。

## 5. UPS schema（`UPS1/2/3`，塔式与机架式同构）

以 `HU10K_manifest.json → telemetry` 为准（其余型号同构）：

```json
{
 "device": "A01.ups2", "ts": 1786200000,
 "mode": "normal | battery | bypass | eco | standby | fault",
 "input":  {"voltage_v": 220.4, "current_a": 12.6, "freq_hz": 50.0, "state": "ok | abnormal"},
 "output": {"voltage_v": 220.1, "current_a": 11.8, "freq_hz": 50.0, "load_pct": 58, "state": "on | off"},
 "battery": {"voltage_v": 192.0, "soc_pct": 86, "current_a": -4.2, "charging": true, "runtime_min": 42},
 "temp_c": 33.5,
 "alarms": []
}
```

### 绑定映射

| 对象 | 字段 | 状态 → 表现 |
|------|------|-------------|
| `{PREFIX}_LED_MAINS` | `input.state` | ok=绿 / abnormal=红 |
| `{PREFIX}_LED_BYPASS` | `mode==bypass` | 是=琥珀 / 否=灭 |
| `{PREFIX}_LED_INVERT` | `mode` | normal/eco=绿 / 其他=灭 |
| `{PREFIX}_LED_BATTERY` | `mode==battery` / `battery.charging` | 放电=琥珀 / 充电=绿闪 |
| `{PREFIX}_LED_FAULT` | `alarms` / `mode==fault` | 有=红 / 无=灭 |
| `{PREFIX}_LED_LOAD_{1..5}` | `output.load_pct` | 段数=ceil(load/20)；≤60%绿 / 60–80%琥珀 / >80%红 |
| `{PREFIX}_LED_BATT_{1..5}` | `battery.soc_pct` | 段数=ceil(soc/20)；>50%绿 / 20–50%琥珀 / <20%红快闪 |
| `{PREFIX}_SCREEN`（LCD Mesh） | 全量 | UE5 替换为动态 UMG/纹理 |

## 6. 机柜 schema（`RACK_42U`）

机柜本身无 LED，遥测体现为**柜级环境/供电视图**（UE5 侧用 UMG 面板 + 机柜 Anchor 定位）：

```json
{
 "device": "A01", "ts": 1786200000,
 "env": {"temp_inlet_c": 22.5, "temp_outlet_c": 31.8, "humidity_pct": 45},
 "pdu": {
  "left":  {"power_w": 2350, "current_a": 10.7, "state": "ok | overload"},
  "right": {"power_w": 2180, "current_a": 9.9,  "state": "ok | overload"}
 },
 "door": {"front": "closed | open", "rear": "closed | open"},
 "units": [{"u": 10, "device": "A01.srv1", "state": "on"}]
}
```

### 绑定映射

| 对象 | 字段 | 说明 |
|------|------|------|
| `PDU_L` / `PDU_R`（custom prop `data_bind`） | `pdu.left/right` | 选中高亮 + 面板读数 |
| `RACK_ANCHOR_U{nn}`（Empty） | `units[i]` | U 位占用标签定位 |
| 柜门对象 `DoorF_*` / `DoorR_*` | `door.*` | 可选：门磁告警高亮 |

## 7. UE5 侧绑定实现要点

1. **导入**：FBX `use_custom_props` 已开启，UE5 导入后 custom props 落在各 Static Mesh 的 user-defined metadata；Empty 锚点导入为 SceneComponent/空 Actor。
2. **LED 驱动**：对 LED Mesh 在 `BeginPlay` 创建 Material Instance Dynamic，订阅遥测总线，按 §3–§5 的状态表设置 Emissive Color/Intensity；闪烁用 TimeLine 或材质参数 + `sin(time)`。
3. **闪烁语义统一**：`activity`=绿闪、`fault`=琥珀闪、低电量=红快闪；频率建议 2Hz（快闪 4Hz），全局同步相位避免视觉噪声。
4. **数据接入**：建议 MQTT/HTTP 轮询 → UE5 `UTelemetrySubsystem`（见 [UE5_ASSEMBLY.md](./UE5_ASSEMBLY.md) §4），按设备 ID 路由到对应 Actor。
