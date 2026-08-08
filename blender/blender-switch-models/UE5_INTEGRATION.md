# 机房国产交换机 3D 资产 — UE5 集成设计文档 + 数据契约

> 资产由 Blender 5.2 参数化脚本 1:1 生成（毫米建模，FBX 导出为 cm，UE5 默认 1 单位=1cm，直接 1:1）。
> 生成管线：`switch_kit.py`（建模库）→ `build_all.py`（三机装配）→ `export_package.py`（导出+渲染），一键可重跑。

## 1. 资产清单

| 文件 | 内容 | 尺寸 (mm) | 端口构成 |
|------|------|-----------|----------|
| `SW1_Huawei_S5735-L24P4S-A1.fbx` | 华为 S5735 接入层 PoE 交换机 | 442×220×43.6 (1U) | 24×RJ45(1G PoE+) + 4×SFP(1G) + MGMT + Console |
| `SW2_H3C_S5130S-52S-EI.fbx` | H3C S5130S 高密度千兆接入交换机 | 440×260×43.6 (1U) | 48×RJ45(1G) + 4×SFP+(10G) |
| `SW3_Huawei_CE6857-48S8CQ.fbx` | 华为 CE6857 数据中心 TOR 交换机 | 442×420×43.6 (1U) | 48×SFP28(25G) + 8×QSFP28(100G) |
| `*_manifest.json` | 每台设备的端口数据契约（见 §4） | — | — |
| `*.glb` | 同内容 glTF（供 Web/Three.js 预览，UE5 不用） | — | — |
| `switch_showcase.blend` | 展台场景源文件（含灯光/相机/辉光合成器） | — | — |

## 2. UE5 导入设置

FBX 导入对话框：

| 选项 | 值 | 说明 |
|------|-----|------|
| Import Uniform Scale | 1.0 | 几何已按 cm 导出，无需缩放 |
| Combine Meshes | **关** | LED/Anchor 必须是独立子对象 |
| Import Materials / Textures | 开 / 关 | 材质仅作占位，正式用 §3 的主材质替换 |
| Generate Lightmap UVs | 关 | 小物件用不到光照贴图（Lumen/动态光照） |

导入后每个 FBX 得到一个 StaticMesh 资产（含子对象层级）。建议建 `Blueprint Actor`（如 `BP_Switch_S5735`）把 StaticMesh 作为根组件。

## 3. 命名规范与 LED 动态变色

### 3.1 对象命名（FBX 子对象名 = 数据契约的键）

| 模式 | 示例 | 说明 |
|------|------|------|
| `{PREFIX}_DETAIL` | `S5735_DETAIL` | 静态合并网格（机箱+端口笼+丝印，1~10 个材质槽） |
| `{PREFIX}_LED_P{nn}_LNK` | `S5735_LED_P01_LNK` | 端口 nn 的 Link 灯（独立 Mesh） |
| `{PREFIX}_LED_P{nn}_ACT` | `S5735_LED_P01_ACT` | 端口 nn 的 Active 灯 |
| `{PREFIX}_LED_SYS_{label}` | `S5735_LED_SYS_PWR` | 系统灯（PWR/SYS/STK） |
| `{PREFIX}_ANCHOR_P{nn}` | `S5735_ANCHOR_P01` | Empty → UE5 中为 SceneComponent，端口 UI 挂点（端口上方 10mm、面板前方 12mm） |

PREFIX：`S5735` / `H3C52` / `CE6857`。

### 3.2 LED 主材质与状态色映射

建一个主材质 `M_LED_Master`：参数 `LedColor` (Vector3, 乘 Emissive)、`Intensity` (Scalar, 默认 6)、`Flicker` (Scalar 0/1)。
运行时对每个 LED 子对象创建 **Dynamic Material Instance**，按状态写参数：

| 状态 | LedColor | 语义 |
|------|----------|------|
| `DOWN` | (0.01, 0.012, 0.014)，Intensity 0.2 | 灭（无连接） |
| `UP` | (0.00, 0.90, 0.35) | 绿 = Link Up |
| `ACTIVE` | (1.00, 0.55, 0.05) + Flicker | 琥珀 = 数据收发（ACT 灯闪烁） |
| `ALARM` | (1.00, 0.08, 0.10) | 红 = 端口告警 |
| `UPLINK` | (0.10, 0.45, 1.00) | 蓝 = 上联指示 |

ACT 闪烁：材质内用 `Time * Flicker` 驱动 Emissive 脉冲，或在蓝图 Timeline 中做 8Hz 方波；无需逐帧改参数。

### 3.3 C++ / 蓝图骨架

```cpp
// 按端口索引取 LED 子对象并改色（Blueprint 同理: Get Child Component by Name）
void ASwitchActor::ApplyPortState(int32 PortIndex, EPortLinkState State, bool bActive)
{
    const FString Prefix = GetPrefix();  // "S5735"
    auto SetLed = [&](const FString& Suffix, const FLinearColor& C, float Intensity)
    {
        const FString Name = FString::Printf(TEXT("%s_LED_P%02d_%s"), *Prefix, PortIndex, *Suffix);
        if (UStaticMeshComponent* Led = FindComponentByName<UStaticMeshComponent>(*Name))
            if (UMaterialInstanceDynamic* Mid = Led->CreateAndSetMaterialInstanceDynamic(0))
            {
                Mid->SetVectorParameterValue(TEXT("LedColor"), C);
                Mid->SetScalarParameterValue(TEXT("Intensity"), Intensity);
            }
    };
    static const TMap<EPortLinkState, FLinearColor> Colors = {
        {EPortLinkState::Down,  FLinearColor(0.01f, 0.012f, 0.014f)},
        {EPortLinkState::Up,    FLinearColor(0.00f, 0.90f, 0.35f)},
        {EPortLinkState::Alarm, FLinearColor(1.00f, 0.08f, 0.10f)},
    };
    SetLed(TEXT("LNK"), Colors[State], State == EPortLinkState::Down ? 0.2f : 6.0f);
    SetLed(TEXT("ACT"), bActive ? FLinearColor(1.0f, 0.55f, 0.05f) : FLinearColor(0.01f,0.012f,0.014f),
           bActive ? 6.0f : 0.2f);
}
```

## 4. 数据契约（manifest JSON）

每台设备一个 `{PREFIX}_manifest.json`。**FBX 对象名 ↔ JSON 记录** 通过 `led_link` / `led_act` / `anchor` 字段一一对应，UE5 侧不需要任何硬编码坐标。

```jsonc
{
  "model": "Huawei CloudEngine S5735-L24P4S-A1",   // 设备型号
  "role": "接入层 PoE 交换机",                     // 机房角色
  "prefix": "S5735",                               // 对象名前缀
  "fbx": "SW1_Huawei_S5735-L24P4S-A1.fbx",
  "dims_mm": [442, 220, 43.6],                     // 宽×深×高
  "ports": [
    {
      "index": 1,                                  // 面板端口号 (1..N)
      "type": "RJ45",                              // RJ45 | SFP | SFP+ | SFP28 | QSFP28 | CONSOLE
      "ifname": "GigabitEthernet0/0/1",            // 设备 CLI 接口名
      "port_type": "RJ45_1G_POE",                  // 物理+速率+特性
      "speed_mbps": 1000,                          // 标称速率 (下行=上行=标称, 全双工)
      "poe": true,                                 // 是否 PoE 供电口
      "purpose": "办公网接入",                      // 用途（示例规划, 运行时可覆盖）
      "vlan": "Access: 20",                        // VLAN 规划
      "led_link": "S5735_LED_P01_LNK",             // → FBX 子对象名
      "led_act":  "S5735_LED_P01_ACT",
      "anchor":   "S5735_ANCHOR_P01",              // → UI 挂点 Empty
      "pos_mm": [-175.0, -112.0, 8.2]              // 端口前面中心 (设备局部坐标, mm)
    }
  ],
  "system_leds": [
    { "label": "PWR", "object": "S5735_LED_SYS_PWR", "pos_mm": [-215, -112, -15.5] }
  ]
}
```

### 运行时状态推送契约（遥测 → 表现层）

```jsonc
// 单端口状态更新 (WebSocket/MQTT/UE5 内结构体均可)
{
  "device": "S5735",              // prefix
  "port": 7,                      // index
  "link": "alarm",                // up | down | alarm
  "active": false,                // 是否有流量 (驱动 ACT 灯)
  "speed_negotiated_mbps": 1000,  // 协商速率（可低于标称）
  "rx_mbps": 12.4,                // 实时下行
  "tx_mbps": 3.1,                 // 实时上行
  "poe_watts": 15.4,              // PoE 实供功率 (非 PoE 口为 null)
  "error": "CRC errors rising"    // 告警原因 (link=alarm 时)
}
```

表现层只依赖 `port`（索引）查 manifest → 找到 LED/Anchor 对象名 → 应用 §3.2 的颜色规则 + 更新 UI 字段。**上下行速率直接显示 `rx_mbps`/`tx_mbps`**。

## 5. 端口属性 UI（科技感 + 体验设计）

1. **挂点**：每个 `ANCHOR_P{nn}` 上挂 `WidgetComponent`（世界空间），渲染端口卡片；平时隐藏， Hover/注视 端口时淡入。锚点已定位在端口上方 10mm、面板前 12mm，正对机柜前的观察者。
2. **Hover 检测**：端口外壳已合并进 `_DETAIL` 网格，不可单独拾取。按 manifest 的 `pos_mm` + 端口类型尺寸（RJ45 15×13.2 / SFP系 14.2×9.6 / QSFP28 18.6×9.8 mm）在运行时生成**透明碰撞盒**（BoxComponent，Visibility 通道），体验上指哪亮哪。
3. **卡片内容**（深色玻璃拟态 + 青色描边，参考 `preview_jack.png` 的观感）：
   - 标题：`ifname` + 状态徽章（UP 绿 / DOWN 灰 / ALARM 红脉冲）
   - 速率：`speed_negotiated_mbps` / 标称 `speed_mbps`，实时 `rx/tx` 双行迷你柱状图
   - 用途 `purpose`、VLAN、PoE 功率（仅 PoE 口显示）
   - ALARM 时底部红色横幅显示 `error`
4. **镜头**：双击交换机 → 平滑聚焦到面板前 40cm；双击端口 → 聚焦到端口并固定卡片；Esc 返回全景。
5. **告警总览**：侧边栏列出全部 `link=alarm` 端口，点击直飞定位。
6. **环境**：暗色机房 + Lumen；LED 是主要点光源观感（Emissive + Bloom 后处理，阈值≈0.8），与 `showcase.png` 的展台观感一致。

## 6. 性能预算

| 项 | 量 | 说明 |
|----|----|------|
| 单设备 Draw Call | ≤ 12 | `_DETAIL` 每材质 1 次 + LED 实例化（可用 ISM/DSM 进一步优化到 ≤4） |
| 单设备三角面 | 3~8 万 | 1U 设备特写级精度，机房 19" 机柜 42U 放 40 台也 < 200 万面 |
| Anchor/碰撞盒 | 纯逻辑开销 | 不参与渲染 |
| LED 动态材质 | 每口 2 个 MID | 仅参数写入，无 shader 编译 |

## 7. 重建与定制

```powershell
# 一键重建全部资产 + 渲染验收图（需 Blender 5.x）
& "D:\Program Files\Blender Foundation\Blender 5.2\blender.exe" -b --python export_package.py
```

- 改端口数量/布局 → `build_all.py` 的 `build_swN()`
- 改端口造型/LED 位置 → `switch_kit.py` 的 `rj45()` / `sfp_cage()`
- 改状态演示（哪些口红/灭）→ `build_all.py` 各 `states` 字典
