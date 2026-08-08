# 国产机柜 PDU 3D 资产 — UE5 集成设计文档 + 数据契约

> 资产由 Blender 5.2 参数化脚本 1:1 生成（毫米建模，FBX 导出为 cm，UE5 默认 1 单位=1cm，直接 1:1）。
> 生成管线：`pdu_kit.py`（建模库）→ `build_all.py`（三机装配）→ `export_package.py`（导出+渲染），一键可重跑。
> 坐标约定与交换机资产一致：前面板朝 -Y；Web 侧 mapPos(mm) = (x, z, -y)。

## 1. 资产清单（调研自国产主流在售型号）

| 文件 | 原型 | 定位 | 尺寸 (mm) | 面板构成 |
|------|------|------|-----------|----------|
| `PDU1_BULL_GNE-1080A.fbx` | 公牛 GNE-1080A | 机柜计量型（电商/集采销量最大） | 440×44.4×44.4 (1U) | 8×GB10A 五孔 + LCD 计量表 + 过载保护开关 + PWR 灯 |
| `PDU2_TOP_TZ-C032.fbx` | 突破 TOP TZ-C032 | 智能监测型（机房集采主流） | 440×60×44.4 (1U) | 8×GB10A（每路 LED）+ LCD + RJ45 网管 + PWR/ALM/NET |
| `PDU3_CLEVER_PFGA-134-0800.fbx` | 克莱沃 MPDU Pro PFGA-134-0800 | 数据中心管理型（运营商/IDC 主流） | 482.4×66×44.4 (1U) | 8×GB10A 防脱扣（每路 LED）+ LCD + RJ45 + 32A 液压断路器 |
| `{PREFIX}_manifest.json` | 每台设备的数据契约（见 §4） | — | — | — |
| `*.glb` | 同内容 glTF（供 Web/Three.js 预览，UE5 不用） | — | — | — |
| `pdu_showcase.blend` | 展台场景源文件（灯光/相机/辉光） | — | — | — |

PREFIX：`BULL` / `TOP` / `PFGA`。

## 2. UE5 导入设置

FBX 导入对话框：

| 选项 | 值 | 说明 |
|------|-----|------|
| Import Uniform Scale | 1.0 | 几何已按 cm 导出 |
| Combine Meshes | **关** | LED/LCD/Anchor 必须是独立子对象 |
| Import Materials / Textures | 开 / 关 | 材质仅占位，正式用 §3 主材质替换 |
| Generate Lightmap UVs | 关 | Lumen/动态光照 |

导入后建议建 `BP_PDU_xxx` Blueprint Actor，StaticMesh 作根组件。

## 3. 命名规范与动态表现

### 3.1 对象命名（FBX 子对象名 = 数据契约的键）

| 模式 | 示例 | 说明 |
|------|------|------|
| `{PREFIX}_DETAIL` | `TOP_DETAIL` | 静态合并网格（机身+插座+丝印） |
| `{PREFIX}_LED_P{nn}` | `TOP_LED_P04` | 插座 nn 的分位状态灯（独立 Mesh；公牛无分位灯，`outlets[].led=null`） |
| `{PREFIX}_LED_SYS_{label}` | `PFGA_LED_SYS_ALM` | 系统灯（PWR 电源 / ALM 告警 / NET 通讯） |
| `{PREFIX}_LCD` | `TOP_LCD` | **数据面板发光屏**（独立 Mesh，平面 UV 全幅 0..1，36×20mm 可视面） |
| `{PREFIX}_ANCHOR_P{nn}` | `TOP_ANCHOR_P04` | Empty → SceneComponent，插座 UI 挂点（上方 10mm、面板前 14mm） |
| `{PREFIX}_ANCHOR_LCD` | `TOP_ANCHOR_LCD` | Empty，电力详情大屏 UI 挂点 |

### 3.2 LED 主材质与状态色映射

主材质 `M_LED_Master`：参数 `LedColor` (Vector3)、`Intensity` (Scalar, 默认 6)、`Flicker` (Scalar 0/1)。
运行时每 LED 建 **Dynamic Material Instance**：

| 插座状态 | LedColor | Intensity | 语义 |
|----------|----------|-----------|------|
| `NORMAL` | (0.00, 0.90, 0.35) | 6 | 绿 = 正常供电（负载 <60%） |
| `HIGH`   | (1.00, 0.55, 0.05) | 6 | 琥珀 = 高负载（60%~85%） |
| `ALARM`  | (1.00, 0.08, 0.10) | 6 + Flicker | 红闪 = 过载/越限告警 |
| `OFF`    | (0.01, 0.012, 0.014) | 0.2 | 灭 = 继电器断开/断路器跳闸 |

系统灯：PWR 常绿；ALM 平时灭、断路器跳闸/总告警时红 + Flicker；NET 蓝（有通讯）。

### 3.3 LCD 数据面板（两种方案，任选）

**方案 A（推荐，科技范）**：`{PREFIX}_LCD` 面片材质改为 Emissive + **RenderTarget**，用一个 UMG `WBP_PDU_LCD` 绘制到底色 #021012、青色 #00E5FF 点阵字体画面（参考 `demo/index.html` 的 Canvas 绘制逻辑），渲染到 512×256 RenderTarget。画面字段：电压 / 电流 / 功率 / 电能 / 负载率条；断路器跳闸时整屏切换红色 `OVERLOAD TRIP` 告警帧（1Hz 闪烁）。

**方案 B（轻量）**：隐藏 `{PREFIX}_LCD`，在 `ANCHOR_LCD` 挂世界空间 `WidgetComponent` 直接显示 UMG 面板。

### 3.4 C++ / 蓝图骨架

```cpp
// 分位灯按负载率变色
void APDUActor::ApplyOutletState(int32 Idx, EOutletState S)
{
    const FString Name = FString::Printf(TEXT("%s_LED_P%02d"), *Prefix, Idx);
    if (UStaticMeshComponent* Led = FindComponentByName<UStaticMeshComponent>(*Name))
        if (UMaterialInstanceDynamic* Mid = Led->CreateAndSetMaterialInstanceDynamic(0))
        {
            static const TMap<EOutletState, FLinearColor> C = {
                {EOutletState::Normal, FLinearColor(0.00f, 0.90f, 0.35f)},
                {EOutletState::High,   FLinearColor(1.00f, 0.55f, 0.05f)},
                {EOutletState::Alarm,  FLinearColor(1.00f, 0.08f, 0.10f)},
                {EOutletState::Off,    FLinearColor(0.01f, 0.012f, 0.014f)},
            };
            Mid->SetVectorParameterValue(TEXT("LedColor"), C[S]);
            Mid->SetScalarParameterValue(TEXT("Intensity"), S == EOutletState::Off ? 0.2f : 6.0f);
            Mid->SetScalarParameterValue(TEXT("Flicker"),  S == EOutletState::Alarm ? 1.0f : 0.0f);
        }
}

// 断路器跳闸: 全分位灯灭 + ALM 红闪 + LCD 切告警帧
void APDUActor::SetBreakerTripped(bool bTripped) { /* 遍历 outlets + LED_SYS_ALM + LCD 状态参数 */ }
```

## 4. 数据契约（manifest JSON）

```jsonc
{
  "model": "TOP 突破 TZ-C032 智能PDU",
  "role": "智能监测型 PDU (网管)",
  "prefix": "TOP",
  "fbx": "PDU2_TOP_TZ-C032.fbx",
  "dims_mm": [440, 60, 44.4],
  "electrical": {
    "input": "220V~/32A IEC 60309 工业插头",
    "rated_a": 32,            // 额定总电流 → 断路器跳闸阈值
    "max_w": 7360,
    "phase": "单相",
    "metering": "分位计量 + SNMP/Modbus 网管"
  },
  "lcd": {
    "object": "TOP_LCD",      // FBX 屏幕对象
    "anchor": "TOP_ANCHOR_LCD",
    "pos_mm": [-166, -32, 0],
    "size_mm": [42, 24]
  },
  "outlets": [
    {
      "index": 1,                     // 面板位号 1..8
      "type": "GB10A",
      "socket": "GB 10A 新国标五孔",
      "rated_a": 10,                  // 分位额定电流 → 负载率分母
      "purpose": "机柜服务器 A 路",     // 用途（运行时可覆盖）
      "led": "TOP_LED_P01",           // → FBX 子对象; 公牛为 null
      "anchor": "TOP_ANCHOR_P01",
      "pos_mm": [-100, -32, 0]        // 插座前面中心 (设备局部, mm)
    }
  ],
  "system_leds": [
    { "label": "PWR", "object": "TOP_LED_SYS_PWR", "pos_mm": [-208, -32, -15.5] }
  ]
}
```

### 运行时遥测推送契约（遥测 → 表现层）

```jsonc
// 整机帧 (1Hz 推送足够; WebSocket/MQTT/UE5 结构体均可)
{
  "device": "TOP",
  "voltage_v": 229.8,
  "current_a": 9.6,           // 总电流 = Σ outlets.a
  "power_kw": 2.20,
  "energy_kwh": 1034.5,       // 累计电能, 单调递增
  "pf": 0.97,                 // 功率因数
  "temp_c": 33.5,             // 机内温度
  "breaker": "closed",        // closed | tripped (tripped 时全分位 OFF + ALM 红闪)
  "outlets": [
    { "index": 1, "on": true, "a": 1.9, "w": 420, "state": "normal" }
    // state: normal | high | alarm | off  —— 由后端按 §3.2 阈值判定, 表现层只消费
  ]
}
```

表现层只依赖 `outlets[].index` 查 manifest → 找到 LED/Anchor 对象名 → 应用 §3.2 颜色规则 + 更新 UI 字段。**LCD 帧直接用整机字段渲染**，无需额外坐标。

## 5. 交互 UI 设计（科技感 + 极致体验）

参考实现：`demo/index.html`（Three.js 完整可跑，UE5 侧按此等价翻译为 UMG）：

1. **挂点**：每个 `ANCHOR_P{nn}` 挂世界空间 Widget，平时隐藏，Hover/注视插座时全息卡片淡入（毛玻璃 + 青描边 + 底部尖角）。
2. **Hover 检测**：插座已合并进 `_DETAIL`，按 manifest `pos_mm` + 模块尺寸（40×32mm）运行时生成透明 BoxComponent（Visibility 通道），指哪亮哪。
3. **全息卡片内容**：`OUT-nn + 用途` 标题 + 状态徽章（NORMAL 绿 / HIGH 琥珀 / ALARM 红脉冲 / OFF 灰）；实时电流/功率双行；**负载率环形条**（按 `w/(rated_a×220)`）；插座制式。
4. **电力仪表盘**（注视 LCD 或双击整机弹出）：弧形功率表（当前/max_w 渐变弧）、电压/电流/PF/温度四位数码管、电能滚动计数、8 路负载频谱柱。
5. **实时波形**：屏幕底部示波器——电压正弦（青）+ 电流相移波（琥珀）双迹滚动，跳闸时波形拉平 + 红色抖动。
6. **继电器远程分合**：点击插座 → 确认后下发 `on=false`（LED 灭、负载归零、事件记录）；再点恢复。**断路器跳闸**（总电流越限）：全分位 OFF、ALM 红闪、LCD 红色告警帧、顶部红色横幅 + 事件流记录，提供「复位断路器」。
7. **镜头**：双击整机 → 平滑聚焦面板前 40cm；双击插座 → 聚焦该位并钉住卡片；Esc 返回全景；聚焦态四角科技取景框。
8. **告警总览**：右侧事件流（时间戳 + 级别色条），点击直飞故障插座。
9. **环境**：暗色机房 + Lumen；LED/LCD 为主要自发光源（Emissive + Bloom 阈值≈0.72），与 `showcase.png` 观感一致。

## 6. 性能预算

| 项 | 量 | 说明 |
|----|----|------|
| 单设备 Draw Call | ≤ 14 | `_DETAIL` 每材质 1 次 + LED/LCD（可用 ISM 进一步合并） |
| 单设备三角面 | 2~5 万 | 特写级精度，42U 机柜双面 20 台 < 100 万面 |
| Anchor/碰撞盒 | 纯逻辑 | 不参与渲染 |
| LED 动态材质 | 每路 1 MID | 仅参数写入 |
| LCD | 1 RT 512×256 @2Hz | 远低于帧率需求 |

## 7. 重建与定制

```powershell
# 一键重建全部资产 + 渲染验收图（headless, 不占用 MCP 会话）
& "D:\Program Files\Blender Foundation\Blender 5.2\blender.exe" -b --factory-startup --python export_package.py
```

- 改插座数量/布局 → `build_all.py` 的 `_layout_outlets`（x0/pitch/wide）
- 改插座造型/孔位 → `pdu_kit.py` 的 `outlet_gb10a()`
- 改 LCD 尺寸 → `lcd_module(w, h)`
- 改状态演示 → `build_all.py` 各 `states` 字典
