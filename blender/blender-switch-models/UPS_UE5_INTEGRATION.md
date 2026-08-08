# 国产主流 UPS 3D 资产 — UE5 集成设计文档 + 数据契约

> 资产由 Blender 5.2 参数化脚本 1:1 生成（毫米建模，FBX 导出为 cm，UE5 默认 1 单位=1cm，直接 1:1）。
> 生成管线：`ups_kit.py`（建模库）→ `build_ups.py`（三机装配 + 演示状态）→ `export_ups.py`（导出+渲染），一键可重跑。
> 选型依据：2026 国内 UPS 市场头部阵营（华为 21.5% / 科华 18.6% / 易事特 / 山特 / 科士达），覆盖塔式与机架式两大形态、6KVA/10KVA 主流功率段。

## 1. 资产清单

| 文件 | 内容 | 尺寸 (mm) | 面板构成 |
|------|------|-----------|----------|
| `UPS1_SANTAK_C6KS.fbx` | 山特 C6KS 城堡系列塔式在线 UPS | 240×500×460 | LCD + 5 状态灯 + 负载/电量段显 + 电源键 |
| `UPS2_HUAWEI_UPS2000-A-10KTTL.fbx` | 华为 UPS2000-A 机架式 2U UPS | 430×585×86 (2U) | LCD + 4 按键 + 5 状态灯 + 负载/电量段显 + 挂耳 |
| `UPS3_KSTAR_YDC9110H.fbx` | 科士达 YDC9110H 塔式 UPS（带脚轮） | 250×590×655 | LCD + 5 状态灯 + 负载/电量段显 + 3 功能键 |
| `{PREFIX}_manifest.json` | 每台设备的数据契约（见 §4） | — | — |
| `*.glb` | 同内容 glTF（供 Web/Three.js 预览，UE5 不用） | — | — |
| `ups_showcase.blend` | 展台场景源文件（含灯光/相机/辉光合成器） | — | — |

PREFIX：`C6KS` / `HU10K` / `YDC10H`。

## 2. UE5 导入设置

FBX 导入对话框：

| 选项 | 值 | 说明 |
|------|-----|------|
| Import Uniform Scale | 1.0 | 几何已按 cm 导出，无需缩放 |
| Combine Meshes | **关** | LED/SCREEN/Anchor 必须是独立子对象 |
| Import Materials / Textures | 开 / 关 | 材质仅作占位，正式用 §3 的主材质替换 |
| Generate Lightmap UVs | 关 | Lumen/动态光照即可 |

导入后建 `Blueprint Actor`（如 `BP_UPS_C6KS`）把 StaticMesh 作为根组件，挂上 §5 的遥测驱动逻辑。

## 3. 命名规范与状态灯动态变色

### 3.1 对象命名（FBX 子对象名 = 数据契约的键）

| 模式 | 示例 | 说明 |
|------|------|------|
| `{P}_DETAIL` | `C6KS_DETAIL` | 静态合并网格（机箱/面板/插座/风扇/丝印） |
| `{P}_SCREEN` | `C6KS_SCREEN` | LCD 发光屏（独立 Mesh，运行时换动态 UI 材质或叠加 WidgetComponent） |
| `{P}_LED_MAINS` | `C6KS_LED_MAINS` | 市电输入状态灯 |
| `{P}_LED_BYPASS` | | 旁路供电状态灯 |
| `{P}_LED_INVERT` | | 逆变工作状态灯 |
| `{P}_LED_BATTERY` | | 电池状态灯 |
| `{P}_LED_FAULT` | | 故障/告警状态灯 |
| `{P}_LED_LOAD_{1..5}` | `C6KS_LED_LOAD_3` | 输出负载段显（**输出电流水平**的可视化） |
| `{P}_LED_BATT_{1..5}` | | 电池电量段显 |
| `{P}_ANCHOR_PANEL` | | Empty → 整机概览卡挂点（面板前方） |
| `{P}_ANCHOR_SCREEN` | | Empty → LCD 大屏 UI 挂点 |
| `{P}_ANCHOR_INPUT` | | Empty → **输入回路**卡挂点（后面板输入端子上方） |
| `{P}_ANCHOR_OUTPUT` | | Empty → **输出回路**卡挂点（后面板输出端子/插座上方） |
| `{P}_ANCHOR_BATT` | | Empty → 电池回路卡挂点 |

### 3.2 状态语义 → LED 颜色映射

主材质 `M_LED_Master`：参数 `LedColor` (Vector3)、`Intensity` (Scalar 默认 6)、`Flicker` (Scalar 0/1)。运行时为每个 LED 子对象建 Dynamic Material Instance，按下表写参：

| 遥测字段 | 条件 | 目标 LED | LedColor | 备注 |
|----------|------|----------|----------|------|
| `input.state` | `ok` | LED_MAINS | (0.00, 0.90, 0.35) 绿 | 市电正常 |
| | `abnormal`（欠压/过压/频率越限） | LED_MAINS | (1.00, 0.08, 0.10) 红 | |
| | `off`（停电） | LED_MAINS | (0.01, 0.012, 0.014) + Intensity 0.2 | 灭 |
| `mode` | `normal`/`eco` | LED_INVERT 绿 | | 在线逆变供电 |
| | `battery` | LED_INVERT 绿 + LED_BATTERY (1.00,0.55,0.05) 琥珀 | | 电池放电中 |
| | `bypass` | LED_BYPASS 琥珀 | | 旁路直供 |
| | `standby` | 全灭，仅屏幕微光 | | |
| | `fault` 或 `alarms` 非空 | LED_FAULT (1.00,0.08,0.10) 红 + Flicker=1 | | 2Hz 闪烁 |
| `output.load_pct` | ≤60% → 点亮段数 N=ceil(pct/20)，色绿 | LED_LOAD_1..N | (0.00,0.90,0.35) | **输出电流大小直接映射段数** |
| | 60~80% | 同上 | (1.00,0.55,0.05) 琥珀 | |
| | >80% | 同上 | (1.00,0.08,0.10) 红 | 过载红灯 |
| `battery.soc_pct` | >50% → N=ceil(soc/20)，色绿 | LED_BATT_1..N | (0.00,0.90,0.35) | |
| | 20~50% | 同上 | (1.00,0.55,0.05) 琥珀 | |
| | <20% | 同上 | (1.00,0.08,0.10) 红 + Flicker | 低电快闪 |
| `battery.charging` | true 且非满 | LED_BATTERY | 绿 + Flicker(慢) | 充电呼吸闪 |

闪烁实现：材质内 `Time * Flicker` 驱动 Emissive 脉冲，或蓝图 Timeline 方波；无需逐帧写参。

### 3.3 C++ 骨架

```cpp
void AUPSActor::ApplyTelemetry(const FUPSTelemetry& T)
{
    auto SetLed = [&](const FString& Name, FLinearColor C, float Intensity, bool bFlicker=false)
    {
        if (UStaticMeshComponent* Led = FindComponentByName<UStaticMeshComponent>(*Name))
            if (auto* Mid = Led->CreateAndSetMaterialInstanceDynamic(0))
            {
                Mid->SetVectorParameterValue(TEXT("LedColor"), C);
                Mid->SetScalarParameterValue(TEXT("Intensity"), Intensity);
                Mid->SetScalarParameterValue(TEXT("Flicker"), bFlicker ? 1.f : 0.f);
            }
    };
    const FLinearColor G(0,.9f,.35f), A(1,.55f,.05f), R(1,.08f,.1f), Off(.01f,.012f,.014f);
    // 市电
    SetLed(Prefix+"_LED_MAINS", T.Input.State==EInputState::Ok?G:(T.Input.State==EInputState::Abnormal?R:Off),
           T.Input.State==EInputState::Off?0.2f:6.f);
    // 模式
    SetLed(Prefix+"_LED_INVERT",  T.Mode==EUPSMode::Standby?Off:G, 6.f);
    SetLed(Prefix+"_LED_BYPASS",  T.Mode==EUPSMode::Bypass?A:Off, T.Mode==EUPSMode::Bypass?6.f:0.2f);
    SetLed(Prefix+"_LED_BATTERY", T.Mode==EUPSMode::Battery?A:(T.bCharging?G:Off), 6.f, T.bCharging);
    SetLed(Prefix+"_LED_FAULT",   T.Mode==EUPSMode::Fault||T.Alarms.Num()>0?R:Off,
           T.Alarms.Num()>0?6.f:0.2f, T.Alarms.Num()>0);
    // 负载段(输出电流水平)
    const int32 N = FMath::CeilToInt(T.Output.LoadPct/20.f);
    const FLinearColor LC = T.Output.LoadPct>80?R:(T.Output.LoadPct>60?A:G);
    for (int32 i=1;i<=5;i++)
        SetLed(FString::Printf(TEXT("%s_LED_LOAD_%d"),*Prefix,i), i<=N?LC:Off, i<=N?6.f:0.2f);
    // 电量段
    const int32 B = FMath::CeilToInt(T.Battery.SocPct/20.f);
    const FLinearColor BC = T.Battery.SocPct<20?R:(T.Battery.SocPct<50?A:G);
    for (int32 i=1;i<=5;i++)
        SetLed(FString::Printf(TEXT("%s_LED_BATT_%d"),*Prefix,i), i<=B?BC:Off, i<=B?6.f:0.2f,
               T.Battery.SocPct<20);
}
```

## 4. 数据契约（manifest JSON + 遥测推送）

### 4.1 manifest（静态，随资产交付）

```jsonc
{
  "model": "SANTAK CASTLE C6KS",
  "prefix": "C6KS",
  "dims_mm": [240, 500, 460],
  "rated": { "kva": 6, "kw": 5.4, "input_v": "120-275VAC", "output_v": "220VAC±2%",
             "batt_vdc": 192, "input_a_max": 32, "output_a_max": 27.3 },
  "leds":   [ { "label": "MAINS", "object": "C6KS_LED_MAINS", "pos_mm": [-88, -253, 326] }, ... ],
  "screen": { "object": "C6KS_SCREEN", "anchor": "C6KS_ANCHOR_SCREEN",
              "pos_mm": [0, -253, 372], "size_mm": [76, 42] },
  "io": {
    "input":  { "anchor": "C6KS_ANCHOR_INPUT",  "desc": "市电输入: 空开+端子排", "pos_mm": [-6, 255, 336] },
    "output": { "anchor": "C6KS_ANCHOR_OUTPUT", "desc": "输出: 国标10A插座x2 + 端子排", "pos_mm": [36, 255, 250] },
    "battery":{ "anchor": "C6KS_ANCHOR_BATT",   "desc": "外接电池组 192VDC", "pos_mm": [92, 255, 250] },
    "comms":  ["RS232", "USB", "EPO", "INT SLOT"]
  }
}
```

**FBX 对象名 ↔ JSON 记录** 通过 `object` / `anchor` 字段一一对应，UE5 侧不需要任何硬编码坐标。

### 4.2 遥测推送（运行时，WebSocket/MQTT/UE5 结构体均可）

```jsonc
{
  "device": "C6KS",
  "ts": 1786200000,
  "mode": "normal",                    // normal|battery|bypass|eco|standby|fault
  "input":  { "voltage_v": 220.4, "current_a": 12.6, "freq_hz": 50.0,
              "state": "ok" },          // ok|abnormal|off  —— 输入电流状态
  "output": { "voltage_v": 220.1, "current_a": 11.8, "freq_hz": 50.0,
              "load_pct": 58, "state": "on" },  // load_pct —— 输出电流水平(驱动 LOAD 段显)
  "battery": { "voltage_v": 192.0, "soc_pct": 86, "current_a": -4.2,
               "charging": true, "runtime_min": 42 },
  "temp_c": 33.5,
  "alarms": []                          // ["overload","battery_low","overtemp","bypass_fail",...]
}
```

表现层只依赖 `device`(prefix) 查 manifest → 找到 LED/Anchor 对象名 → 应用 §3.2 规则 + 更新 UI 字段。**输入电流看 `input.current_a`/`state`，输出电流看 `output.current_a`/`load_pct`**，与真机 LCD 显示项一一对应。

## 5. 状态 UI（科技感 + 极致体验设计）

设计目标：**3 米外一眼知状态，30cm 特写读全部参数**；平时克制，异常时主动引导视线。

1. **整机概览卡**（挂 `ANCHOR_PANEL`，世界空间 Widget，常显但低亮度）：
   - 品牌型号 + 模式徽章（NORMAL 绿 / BATTERY 琥珀 / BYPASS 琥珀 / FAULT 红脉冲）
   - **能量流向图**：`市电输入 → [整流/逆变] → 负载输出` 三段流动虚线，电池支路在下方；哪条路激活哪条流动发光（电池模式时输入段变灰、电池支路亮起）——这是"科技范"的核心视觉
   - 输入 V/A/Hz · 输出 V/A/负载% · 电池 SOC/续航，三列大数字等宽字体， cyan 描边深色玻璃拟态
2. **LCD 大屏**（挂 `ANCHOR_SCREEN`）：把 `{P}_SCREEN` 材质换成动态材质（RenderTarget）或叠加平面 Widget，1:1 复刻真机 LCD 内容：输入/输出参数 + 负载/电量条 + 告警图标；注视屏幕时放大为完整详情页
3. **输入/输出回路卡**（挂 `ANCHOR_INPUT`/`ANCHOR_OUTPUT`，Hover 后面板对应区域时浮现）：
   - 输入卡：电压/电流/频率/功率因数 + 市电波形迷你图（正常=正弦青线，异常=红畸变线）
   - 输出卡：电压/电流/负载率/峰值比 + 已挂负载清单；负载率 >80% 卡片边缘泛红
4. **段显联动**：模型上的 LOAD/BATT 段显颜色与 UI 完全同步（同一状态源），形成"近看模型灯、远看 HUD"的双层信息
5. **告警体验**：FAULT 时整机红色轮廓脉冲（后处理 Outline）+ 镜头轻微推近 + 侧边栏告警列表点击直飞定位；静音按钮可停闪烁但保留红色常亮
6. **镜头**：双击机身 → 平滑聚焦面板前 50cm；双击后面板输入/输出区 → 绕到背面聚焦对应端子；Esc 返回全景。Hover 任意 LED 有 tooltip 解释语义（"MAINS：市电输入正常"）
7. **环境**：暗色机房 + Lumen；LED 是主要点光源观感（Emissive + Bloom 阈值≈0.8），与 `ups_showcase.png` 一致
8. **无障碍/细节**：所有状态色同时配文字/图标（不单独依赖颜色）；数字刷新做 300ms 插值动画；遥测断流 5s 未更新时 UI 降饱和显示"数据延迟"

## 6. 性能预算

| 项 | 量 | 说明 |
|----|----|------|
| 单设备 Draw Call | ≤ 8 | `_DETAIL` 每材质 1 次 + 15 个 LED 实例化 + SCREEN |
| 单设备三角面 | 2~4 万 | 特写级精度，一机房 20 台 < 80 万面 |
| Anchor | 纯逻辑 | 不参与渲染 |
| LED 动态材质 | 每台 15 个 MID | 仅参数写入，无 shader 编译 |

## 7. 重建与定制

```powershell
# 一键重建全部资产 + 渲染验收图（需 Blender 5.x，headless 可跑）
& "D:\Program Files\Blender Foundation\Blender 5.2\blender.exe" -b --python export_ups.py
```

- 改面板布局/端口位置 → `build_ups.py` 的 `build_upsN()`
- 改 LCD/段显/插座造型 → `ups_kit.py` 的 `lcd()` / `seg_bar()` / `cn_socket()`
- 改演示状态（哪些灯亮、段显几格）→ `build_ups.py` 各 build 函数的 `labels`/`lit` 参数
- Web 预览：`demo/ups.html`（Three.js + 遥测模拟器，浏览器直连 GLB）
