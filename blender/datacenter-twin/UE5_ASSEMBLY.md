# UE5_ASSEMBLY.md — 数据中心数字孪生 UE5 装配指南

> 输入资产：`blender-rack/models/RACK_42U.fbx`、`server-3d/SRV_{1U,2U,4U}.fbx`、
> `blender-switch-models/models/{SW1..3, UPS1..3}.fbx`、`datacenter-twin/models/ROOM_ENV.fbx`。
> 契约文件：[datacenter_layout.json](./datacenter_layout.json)（排布）、[TELEMETRY.md](./TELEMETRY.md)（数据绑定）。

---

## 1. 导入设置

| 项 | 值 | 说明 |
|----|----|----|
| Import Uniform Scale | 1.0 | FBX 已按 cm 导出，UE5 默认 1:1 实物尺寸 |
| Convert Scene | 勾 | Blender Z-up → UE5 坐标自动转换 |
| Import Custom Properties | **必勾** | `hw_*` / `led_type` / `data_bind` 元数据是数据绑定的载体 |
| Combine Meshes | 否 | 每硬件对象需独立（选中高亮/LED 动态材质） |
| Import Empty as | Scene Component / Actor | `*_ANCHOR_*` Empty 是 UI 挂点与装配定位点 |

建议导入结构：

```
Content/
  DC/Room/ROOM_ENV
  DC/Rack/RACK_42U
  DC/Server/SRV_1U, SRV_2U, SRV_4U
  DC/Network/SW1_S5735, SW2_H3C52, SW3_CE6857
  DC/Power/UPS1_C6KS, UPS2_HU10K, UPS3_YDC10H
```

## 2. 装配流程（按 datacenter_layout.json）

1. **放房间**：`ROOM_ENV` 置于世界原点。机柜吸附点 `ROOM_ANCHOR_RACK_{id}` 与 `racks[].position` 一一对应。
2. **放机柜**：每个 `racks[]` 实例化 `BP_Rack`（见 §3），Transform 取 `position` / `rotation_z_deg`。
3. **装设备**：对 `units[]` 每项实例化设备 Actor，挂到机柜 Actor 下，局部坐标：
   ```
   local.z = 0.115 + (u-1) * 0.04445        // u_bottom_z，资产单位米 → UE5 cm ×100
   local.y = -0.45 + depth/2                 // 前面板贴前导轨（front=-Y）
   local.x = 0
   ```
   机柜内 `RACK_ANCHOR_U{nn}` Empty 已标在每 U 中心，可直接吸附。
4. **塔式设备**：`floor_items[]` 按 `position` 落地（z=0）。
5. **校验**：设备 19" 耳板（0.4826m）应卡入两前导轨（|x|=0.2326m 中心距）之间；穿模检查用 `preview_rack_*.png` 对比朝向（front=-Y，玻璃门朝冷通道）。

## 3. 蓝图结构建议

```
BP_Room            // 房间壳，持有 Rack Actor 数组
BP_Rack            // 根 SceneComponent = RACK_42U_ROOT
  ├─ Devices[]     // 设备 Actor 引用, key = device_id
  ├─ UAnchors[]    // 42 个 U 位 SceneComponent
  └─ RackPanel(UMG)// 柜级环境/PDU 读数, 绑定 A01.env / A01.pdu.*
BP_DeviceBase      // 所有设备基类
  ├─ DeviceId (FName, 如 A01.srv1)
  ├─ LedBindings TMap<FName, UMeshComponent*>  // data_bind → LED mesh
  ├─ OnTelemetry(FTelemetryMsg)                // 更新 MID + 悬停 UI 数据
  └─ AnchorMap TMap<FName, USceneComponent*>   // 端口 UI 挂点
BP_Server / BP_Switch / BP_UPS  // 继承 BP_DeviceBase, 填各 schema 状态表
```

## 4. 遥测组件骨架（`UTelemetryComponent`）

```cpp
UCLASS(ClassGroup=(DC), meta=(BlueprintSpawnableComponent))
class UTelemetryComponent : public UActorComponent {
    GENERATED_BODY()
public:
    UPROPERTY(EditAnywhere) FName DeviceId;          // "A01.srv1"
    UPROPERTY() TMap<FName, UMeshComponent*> Leds;   // bind path -> LED

    void BeginPlay() override {
        // 1) 遍历子 Mesh, 读 AssetUserData 的 data_bind, 建 Leds 映射
        // 2) 对每个 LED 创建 MID: Mesh->CreateDynamicMaterialInstance(0)
        // 3) 订阅遥测总线: UTelemetrySubsystem::Get()->OnMessage.AddUObject(...)
    }

    void ApplyState(const FString& Bind, const FString& State) {
        // 按 TELEMETRY.md §3-§5 状态表:
        //   FLinearColor C = StateColor(Bind, State);   // 绿/琥珀/红/蓝/灭
        //   MID->SetVectorParameterValue("Emissive Color", C);
        //   闪烁类状态 → 注册到 FlickerSet, Tick 中以 2Hz 方波调 Intensity
    }
};
```

数据源：MQTT（推荐，话题 = 设备 ID）或 HTTP 轮询（5s）；JSON 直接对应 TELEMETRY.md 各 schema。

## 5. 材质清单（LED 统一语义）

| 材质 | 语义 | UE5 处理 |
|------|------|----------|
| `M_LED_Power/Health/Alert/UID/NIC/Disk`（服务器） | 见 TELEMETRY.md §3 | 每实例 MID，按状态改 Emissive |
| `M_LED_GREEN/AMBER/RED/BLUE/OFF`（交换机/UPS） | 见各 manifest `led_semantics` | 做成 5 个母材质 + 参数化闪烁 |
| `M_LightPanel`（房间灯盘） | 自发光 | 可替换为 Rect Light 降低烘焙成本 |
| 其余 `MAT_*`/`M_*` | 静态 PBR | 直接导入，无需动态实例 |

## 6. 验收清单

- [ ] 房间/机柜/设备比例 1:1（机柜高 1.982m ≈ 角色身高 1.8m 参照）
- [ ] 设备前面板朝向冷通道（-Y），玻璃门可透视内部
- [ ] 每台设备 `device_id` 与 layout JSON 一致；遥测模拟推送后 LED 变色正确
- [ ] 悬停端口显示 `ifname`/速率（数据来自各 manifest）
- [ ] U 位占用面板与 `units[]` 一致；PDU 读数随负载变化
