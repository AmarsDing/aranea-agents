# -*- coding: utf-8 -*-
"""
27寸 16:9 显示器 1:1 建模脚本（Blender 4.x）
在 Blender「脚本」工作区打开本文件并点击「运行脚本」：
  1. 按真实尺寸建模（单位：米）
  2. 屏幕为独立物体 + 独立材质槽 M_Screen（UE5 动态材质槽位 / Demo 换图目标）
  3. 电源指示灯独立物体 + M_LED 材质槽（UE5 可控亮灭/变色）
  4. 导出三份产物：
     - models/Monitor_27inch.glb  → 浏览器 Three.js 演示（demo/index.html）
     - models/SM_Monitor.fbx      → UE5 导入（单位自动转 cm，1:1）
     - models/Monitor_manifest.json → 资产清单

屏幕朝向：-Y（Blender 正面）→ glTF 导出后为 +Z（Three.js 正面）。
UE5 导入时如需朝向 +X，设置 Yaw = 90。
"""

import bpy
import math
import json
import os

# ========== 输出路径 ==========
BASE_DIR   = os.path.dirname(os.path.abspath(__file__))
MODELS_DIR = os.path.join(BASE_DIR, "models")
GLB_PATH   = os.path.join(MODELS_DIR, "Monitor_27inch.glb")
FBX_PATH   = os.path.join(MODELS_DIR, "SM_Monitor.fbx")
BLEND_PATH = os.path.join(MODELS_DIR, "monitor_27.blend")
MANIFEST   = os.path.join(MODELS_DIR, "Monitor_manifest.json")

# ========== 0. 清空场景 & 单位 ==========
bpy.ops.object.select_all(action='SELECT')
bpy.ops.object.delete(use_global=False)

scene = bpy.context.scene
scene.unit_settings.system = 'METRIC'
scene.unit_settings.scale_length = 1.0  # 1 单位 = 1 米

collection = bpy.data.collections.new("Monitor")
scene.collection.children.link(collection)

def move_to_collection(obj):
    for c in list(obj.users_collection):
        c.objects.unlink(obj)
    collection.objects.link(obj)

# ========== 1. 材质 ==========
def _set_input(node, names, value):
    """兼容 Blender 3.6 / 4.x 的输入名差异"""
    for n in names:
        if n in node.inputs:
            node.inputs[n].default_value = value
            return

def make_material(name, base_color, metallic=0.0, roughness=0.5,
                  emission=None, emission_strength=0.0):
    mat = bpy.data.materials.new(name)
    mat.use_nodes = True
    bsdf = mat.node_tree.nodes["Principled BSDF"]
    _set_input(bsdf, ["Base Color"], base_color)
    _set_input(bsdf, ["Metallic"], metallic)
    _set_input(bsdf, ["Roughness"], roughness)
    if emission is not None:
        _set_input(bsdf, ["Emission Color", "Emission"], emission)
        _set_input(bsdf, ["Emission Strength"], emission_strength)
    return mat

# 深灰磨砂塑料（边框/背壳）
M_Body  = make_material("M_Body", (0.02, 0.02, 0.022, 1.0), metallic=0.1, roughness=0.4)
# 支架金属（阳极氧化铝）
M_Stand = make_material("M_Stand", (0.35, 0.36, 0.38, 1.0), metallic=0.9, roughness=0.3)
# 屏幕（关键槽位：UE5 替换为动态材质；Demo 中替换贴图）
M_Screen = make_material("M_Screen", (0.005, 0.005, 0.006, 1.0), roughness=0.15,
                         emission=(1.0, 1.0, 1.0, 1.0), emission_strength=3.0)
# 电源指示灯（独立槽位，UE5 可控）
M_LED   = make_material("M_LED", (0.01, 0.01, 0.01, 1.0), roughness=0.3,
                        emission=(0.75, 0.9, 1.0, 1.0), emission_strength=8.0)

# ========== 2. 建模辅助 ==========
def add_beveled_box(name, dims, loc, bevel, material):
    """创建带圆角的立方体，应用所有变换（保证导出尺寸正确）"""
    bpy.ops.mesh.primitive_cube_add(size=1.0, location=loc)
    obj = bpy.context.active_object
    obj.name = name
    obj.dimensions = dims
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)

    if bevel > 0:
        mod = obj.modifiers.new("Bevel", 'BEVEL')
        mod.width = bevel
        mod.segments = 3
        mod.limit_method = 'ANGLE'
        bpy.context.view_layer.objects.active = obj
        bpy.ops.object.modifier_apply(modifier=mod.name)
        for p in obj.data.polygons:
            p.use_smooth = True
        wn = obj.modifiers.new("WeightedNormal", 'WEIGHTED_NORMAL')
        wn.keep_sharp = True
        bpy.ops.object.modifier_apply(modifier=wn.name)

    obj.data.materials.append(material)
    move_to_collection(obj)
    return obj

# ========== 3. 显示器各部件（真实尺寸，米） ==========
# 27寸 16:9 可视区：597.6mm × 336.2mm
PANEL_W, PANEL_H, PANEL_D = 0.613, 0.362, 0.022   # 含边框整机
SCREEN_W, SCREEN_H        = 0.598, 0.336          # 可视区
PANEL_BOTTOM_Z            = 0.140                 # 屏幕下沿距桌面
PANEL_CZ = PANEL_BOTTOM_Z + PANEL_H / 2.0

# --- 前面板（边框+背壳一体，屏幕朝向 -Y） ---
add_beveled_box("SM_Monitor_Body",
                (PANEL_W, PANEL_D, PANEL_H),
                (0, 0, PANEL_CZ), bevel=0.004, material=M_Body)

# --- 背部凸起（驱动板/电路仓） ---
add_beveled_box("SM_Monitor_Housing",
                (0.340, 0.022, 0.200),
                (0, PANEL_D / 2 + 0.010, PANEL_CZ - 0.02), bevel=0.010, material=M_Body)

# --- 屏幕显示面（独立物体，UV 0-1 全覆盖，法线朝 -Y） ---
bpy.ops.mesh.primitive_plane_add(size=1.0,
                                 location=(0, -PANEL_D / 2 - 0.0008, PANEL_CZ),
                                 rotation=(math.radians(90), 0, 0))
screen = bpy.context.active_object
screen.name = "SM_Monitor_Screen"
screen.dimensions = (SCREEN_W, SCREEN_H, 1.0)
bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
screen.data.materials.append(M_Screen)
move_to_collection(screen)
# primitive_plane 自带 0-1 UV；glTF 导出器自动翻转 V，Three.js 中 flipY=false 即为正向

# --- 电源指示灯（下边框右侧，独立物体） ---
add_beveled_box("SM_Monitor_LED",
                (0.012, 0.003, 0.0025),
                (0.280, -PANEL_D / 2 - 0.001, PANEL_BOTTOM_Z + 0.0065),
                bevel=0.001, material=M_LED)

# --- 支架颈 ---
NECK_Z0, NECK_Z1 = 0.018, PANEL_CZ + 0.05
add_beveled_box("SM_Monitor_Neck",
                (0.110, 0.045, NECK_Z1 - NECK_Z0),
                (0, 0.045, (NECK_Z0 + NECK_Z1) / 2.0), bevel=0.008, material=M_Stand)

# --- 底座 ---
add_beveled_box("SM_Monitor_Base",
                (0.240, 0.200, 0.018),
                (0, 0.030, 0.009), bevel=0.006, material=M_Stand)

# ========== 4. 保存 .blend 并导出 GLB + FBX + manifest ==========
os.makedirs(MODELS_DIR, exist_ok=True)
bpy.ops.wm.save_as_mainfile(filepath=BLEND_PATH)

bpy.ops.object.select_all(action='DESELECT')
for obj in collection.objects:
    obj.select_set(True)

# GLB → 浏览器演示
bpy.ops.export_scene.gltf(
    filepath=GLB_PATH,
    export_format='GLB',
    use_selection=True,
    export_apply=True,
    export_yup=True,
)

# FBX → UE5（米→厘米自动转换，导入即 1:1）
bpy.ops.export_scene.fbx(
    filepath=FBX_PATH,
    use_selection=True,
    apply_unit_scale=True,
    global_scale=1.0,
    use_custom_normals=True,
    use_mesh_modifiers=True,
    mesh_smooth_type='FACE',
    add_leaf_bones=False,
    bake_anim=False,
    object_types={'MESH'},
)

# manifest → Demo / UE5 集成说明数据
manifest = {
    "name": "27inch Office Monitor 16:9",
    "scale": "1:1 (meters)",
    "overall_mm": {"width": 613, "height": 502, "depth": 200},
    "panel_mm": {"width": 613, "height": 362, "thickness": 22},
    "screen_mm": {"width": 598, "height": 336, "diagonal_inch": 27},
    "screen_mesh": "SM_Monitor_Screen",
    "screen_material_slot": "M_Screen",
    "led_mesh": "SM_Monitor_LED",
    "led_material_slot": "M_LED",
    "parts": ["SM_Monitor_Body", "SM_Monitor_Housing", "SM_Monitor_Screen",
              "SM_Monitor_LED", "SM_Monitor_Neck", "SM_Monitor_Base"],
    "files": {"glb": "Monitor_27inch.glb", "fbx": "SM_Monitor.fbx"},
    "ue5": {
        "screen_texture_param": "ScreenTexture",
        "screen_power_param": "ScreenOn",
        "note": "对 Screen 组件 Create Dynamic Material Instance 后 Set Scalar/Texture Parameter 即可控制亮灭与画面"
    }
}
with open(MANIFEST, "w", encoding="utf-8") as f:
    json.dump(manifest, f, ensure_ascii=False, indent=2)

print("=" * 60)
print("完成！")
print("GLB:      " + GLB_PATH)
print("FBX:      " + FBX_PATH)
print("Blend:    " + BLEND_PATH)
print("Manifest: " + MANIFEST)
print("=" * 60)
