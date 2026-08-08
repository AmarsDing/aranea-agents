# -*- coding: utf-8 -*-
"""switch_kit.py — 国产机房交换机 1:1 参数化建模工具库
单位: 米 (Blender 标准), 尺寸以毫米常量书写, 乘 MM 转换。
产出: 每台交换机一个 Collection, 含:
  - {PREFIX}_CHASSIS / _PANEL / _DETAIL / _REAR / _LOGO (静态合并网格)
  - {PREFIX}_LED_P{nn}_LNK / _ACT  (每端口独立 LED 对象, UE5 动态材质实例改色)
  - {PREFIX}_LED_SYS_{label}       (系统灯)
  - {PREFIX}_ANCHOR_P{nn}          (Empty, UE5 UI 挂点, 位置=端口上方)
导出: FBX (MESH+EMPTY, 含 modifier 应用) + manifest JSON (端口清单/数据契约)
"""
import bpy
import math
import json
import os

MM = 0.001
ROOT = os.path.dirname(os.path.abspath(__file__))
OUT_DIR = os.path.join(ROOT, "models")
os.makedirs(OUT_DIR, exist_ok=True)

# ---------------------------------------------------------------- 材质

def _mat(name, color, metallic=0.0, rough=0.45, emission=None, estr=0.0):
    m = bpy.data.materials.get(name)
    if not m:
        m = bpy.data.materials.new(name)
        m.use_nodes = True
    b = m.node_tree.nodes["Principled BSDF"]
    b.inputs["Base Color"].default_value = (*color, 1.0)
    b.inputs["Metallic"].default_value = metallic
    b.inputs["Roughness"].default_value = rough
    if emission is not None:
        b.inputs["Emission Color"].default_value = (*emission, 1.0)
        b.inputs["Emission Strength"].default_value = estr
    return m


def init_materials():
    return {
        "chassis":  _mat("M_Chassis",  (0.040, 0.044, 0.052), 0.30, 0.55),
        "panel":    _mat("M_Panel",    (0.026, 0.029, 0.036), 0.40, 0.50),
        "plastic":  _mat("M_Plastic",  (0.010, 0.011, 0.014), 0.10, 0.55),
        "cavity":   _mat("M_Cavity",   (0.002, 0.002, 0.003), 0.00, 1.00),
        "gold":     _mat("M_Gold",     (0.830, 0.640, 0.200), 1.00, 0.22),
        "cage":     _mat("M_Cage",     (0.300, 0.320, 0.360), 0.95, 0.30),
        "fin":      _mat("M_Fin",      (0.110, 0.120, 0.140), 0.75, 0.50),
        "vent":     _mat("M_Vent",     (0.006, 0.007, 0.009), 0.20, 0.70),
        "label":    _mat("M_Label",    (0.550, 0.590, 0.630), 0.30, 0.50),
        "accent":   _mat("M_Accent",   (0.015, 0.160, 0.260), 0.60, 0.30),
        # LED: 绿=LinkUp 琥珀=Active 红=Alarm 蓝=Uplink 灭=Down
        "led_green": _mat("M_LED_GREEN", (0.000, 0.050, 0.020), 0.0, 0.35, (0.00, 0.90, 0.35), 6.0),
        "led_amber": _mat("M_LED_AMBER", (0.050, 0.030, 0.000), 0.0, 0.35, (1.00, 0.55, 0.05), 6.0),
        "led_red":   _mat("M_LED_RED",   (0.050, 0.005, 0.008), 0.0, 0.35, (1.00, 0.08, 0.10), 6.0),
        "led_blue":  _mat("M_LED_BLUE",  (0.005, 0.015, 0.050), 0.0, 0.35, (0.10, 0.45, 1.00), 6.0),
        "led_off":   _mat("M_LED_OFF",   (0.018, 0.020, 0.022), 0.0, 0.40, (0.010, 0.012, 0.014), 0.2),
    }


# ---------------------------------------------------------------- 基础件

def _link(coll, obj):
    for c in list(obj.users_collection):
        c.objects.unlink(obj)
    coll.objects.link(obj)


def box(coll, name, loc, size, mat, bevel=0.0, static=True):
    bpy.ops.mesh.primitive_cube_add(location=loc)
    o = bpy.context.active_object
    o.name = name
    o.scale = (size[0] / 2, size[1] / 2, size[2] / 2)
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    if mat:
        o.data.materials.append(mat)
    if bevel > 0:
        mod = o.modifiers.new("Bev", "BEVEL")
        mod.width = min(bevel, min(size) * 0.45)
        mod.segments = 2
        bpy.ops.object.modifier_apply(modifier=mod.name)
    o["_static"] = static
    _link(coll, o)
    return o


def cyl(coll, name, loc, radius, depth, mat, rot=(0, 0, 0), verts=24, static=True):
    bpy.ops.mesh.primitive_cylinder_add(vertices=verts, radius=radius, depth=depth,
                                        location=loc, rotation=rot)
    o = bpy.context.active_object
    o.name = name
    if mat:
        o.data.materials.append(mat)
    o["_static"] = static
    _link(coll, o)
    return o


def anchor(coll, name, loc):
    o = bpy.data.objects.new(name, None)
    o.empty_display_size = 3 * MM
    o.location = loc
    coll.objects.link(o)
    return o


def label_text(coll, name, body, loc, size_mm, mat, extrude=0.12 * MM, static=True):
    cu = bpy.data.curves.new(name + "_cu", "FONT")
    cu.body = body
    cu.size = size_mm * MM
    cu.extrude = extrude
    cu.bevel_depth = 0
    cu.align_x = "LEFT"
    cu.align_y = "CENTER"
    ob = bpy.data.objects.new(name, cu)
    coll.objects.link(ob)
    ob.location = loc
    ob.rotation_euler = (math.pi / 2, 0, 0)  # 面向 -Y (前面板)
    if mat:
        ob.data.materials.append(mat)
    bpy.context.view_layer.objects.active = ob
    ob.select_set(True)
    bpy.ops.object.convert(target="MESH")
    ob.select_set(False)
    ob["_static"] = static
    return ob


# ---------------------------------------------------------------- 端口件

def rj45(coll, prefix, idx, cx, cy, cz, M, led_mode="green", leds=True):
    """RJ45 电口: 外壳 15.0W x 13.2H, 前面朝 -Y, (cx,cy,cz)=端口前面中心。
    返回 registry entry。"""
    W, H, D = 15.0 * MM, 13.2 * MM, 12.0 * MM
    n = f"{prefix}_J{idx:02d}"
    box(coll, n + "_housing", (cx, cy + D / 2 - 0.5 * MM, cz), (W, D, H), M["plastic"], bevel=0.6 * MM)
    # 前面开口(深色, 前凸 0.2mm 避免被外壳面遮挡)
    box(coll, n + "_cavity", (cx, cy - 0.6 * MM, cz + 0.4 * MM),
        (W - 1.6 * MM, 1.6 * MM, H - 2.4 * MM), M["cavity"])
    # 8pin 金手指条 (开口内顶部, 微凸出开口面 0.05mm 可见)
    box(coll, n + "_gold", (cx, cy - 1.2 * MM, cz + H / 2 - 2.2 * MM),
        (W - 4.2 * MM, 0.5 * MM, 1.6 * MM), M["gold"])
    # 锁扣缺口 (底部, 微凸出开口面)
    box(coll, n + "_latch", (cx, cy - 1.2 * MM, cz - H / 2 + 1.0 * MM),
        (W - 5.0 * MM, 0.5 * MM, 1.2 * MM), M["plastic"])
    # LED: 顶角嵌入 LNK(左)/ACT(右), 前凸于外壳面
    lw, lh = 2.2 * MM, 1.6 * MM
    lz = cz + H / 2 - 2.6 * MM
    ly = cy - 1.0 * MM
    if leds:
        lnk_mat = M["led_green"] if led_mode == "green" else (M["led_red"] if led_mode == "red" else M["led_off"])
        act_mat = M["led_amber"] if led_mode == "green" else M["led_off"]
        lnk = box(coll, f"{prefix}_LED_P{idx:02d}_LNK", (cx - 5.6 * MM, ly, lz), (lw, 1.0 * MM, lh), lnk_mat, bevel=0.3 * MM, static=False)
        act = box(coll, f"{prefix}_LED_P{idx:02d}_ACT", (cx + 5.6 * MM, ly, lz), (lw, 1.0 * MM, lh), act_mat, bevel=0.3 * MM, static=False)
        anc = anchor(coll, f"{prefix}_ANCHOR_P{idx:02d}", (cx, cy - 12 * MM, cz + H / 2 + 10 * MM))
        return {"index": idx, "type": "RJ45", "led_link": lnk.name, "led_act": act.name,
                "anchor": anc.name, "pos_mm": [round(cx / MM, 2), round(cy / MM, 2), round(cz / MM, 2)]}
    anc = anchor(coll, f"{prefix}_ANCHOR_P{idx:02d}", (cx, cy - 12 * MM, cz + H / 2 + 10 * MM))
    return {"index": idx, "type": "CONSOLE", "led_link": None, "led_act": None,
            "anchor": anc.name, "pos_mm": [round(cx / MM, 2), round(cy / MM, 2), round(cz / MM, 2)]}


def sfp_cage(coll, prefix, idx, cx, cy, cz, M, kind="SFP", led_mode="green"):
    """SFP/SFP+/SFP28 光口笼: 14.2W x 9.6H。QSFP28: 18.6W x 9.8H。"""
    if kind == "QSFP28":
        W, H = 18.6 * MM, 9.8 * MM
    else:
        W, H = 14.2 * MM, 9.6 * MM
    D = 14.0 * MM
    n = f"{prefix}_C{idx:02d}"
    box(coll, n + "_cage", (cx, cy + D / 2 - 0.5 * MM, cz), (W, D, H), M["cage"], bevel=0.4 * MM)
    box(coll, n + "_inner", (cx, cy - 0.6 * MM, cz),
        (W - 1.8 * MM, 1.6 * MM, H - 1.8 * MM), M["cavity"])
    # 笼顶散热簧片
    box(coll, n + "_spring", (cx, cy + 3.0 * MM, cz + H / 2 + 0.5 * MM),
        (W - 1.0 * MM, 8.0 * MM, 0.8 * MM), M["fin"])
    lw, lh = 2.0 * MM, 1.4 * MM
    lz = cz + H / 2 + 3.2 * MM
    lnk_mat = M["led_green"] if led_mode == "green" else (M["led_red"] if led_mode == "red" else M["led_off"])
    act_mat = M["led_amber"] if led_mode == "green" else M["led_off"]
    lnk = box(coll, f"{prefix}_LED_P{idx:02d}_LNK", (cx - 3.2 * MM, cy - 0.6 * MM, lz), (lw, 1.0 * MM, lh), lnk_mat, bevel=0.25 * MM, static=False)
    act = box(coll, f"{prefix}_LED_P{idx:02d}_ACT", (cx + 3.2 * MM, cy - 0.6 * MM, lz), (lw, 1.0 * MM, lh), act_mat, bevel=0.25 * MM, static=False)
    anc = anchor(coll, f"{prefix}_ANCHOR_P{idx:02d}", (cx, cy - 12 * MM, cz + H / 2 + 10 * MM))
    return {"index": idx, "type": kind, "led_link": lnk.name, "led_act": act.name,
            "anchor": anc.name, "pos_mm": [round(cx / MM, 2), round(cy / MM, 2), round(cz / MM, 2)]}


def usb(coll, prefix, cx, cy, cz, M):
    box(coll, f"{prefix}_USB", (cx, cy + 2.5 * MM, cz), (13.0 * MM, 6.0 * MM, 5.6 * MM), M["plastic"], bevel=0.4 * MM)
    box(coll, f"{prefix}_USB_in", (cx, cy - 0.8 * MM, cz), (11.4 * MM, 1.6 * MM, 4.0 * MM), M["cavity"])


# ---------------------------------------------------------------- 机箱

def chassis(coll, prefix, w, d, h, M, brand, model):
    """1U 机箱 + 前面板 + 挂耳 + 侧/顶散热 + 后面板(电源/风扇) + 铭牌。返回前面板 y 坐标。"""
    yf = -d / 2  # 前面板基准
    # 主箱体 (前留 3mm 面板位)
    box(coll, f"{prefix}_CHASSIS", (0, 1.5 * MM, 0), (w, d - 3 * MM, h), M["chassis"], bevel=2.0 * MM)
    # 前面板 (微凸 2mm)
    box(coll, f"{prefix}_PANEL", (0, yf - 1.0 * MM, 0), (w - 4 * MM, 2.0 * MM, h - 4 * MM), M["panel"], bevel=1.2 * MM)
    # 挂耳
    for sx in (-1, 1):
        box(coll, f"{prefix}_EAR_{'L' if sx < 0 else 'R'}",
            (sx * (w / 2 + 8 * MM), yf + 1 * MM, 0), (16 * MM, 4 * MM, h), M["chassis"], bevel=0.8 * MM)
        for sz in (-1, 1):
            cyl(coll, f"{prefix}_EARH_{sx}_{sz}",
                (sx * (w / 2 + 8 * MM), yf - 1.2 * MM, sz * (h / 2 - 8 * MM)),
                2.2 * MM, 1.0 * MM, M["cavity"], rot=(math.pi / 2, 0, 0))
    # 顶部散热孔阵 (右侧区域, 细长孔 8x2 排)
    for i in range(8):
        for j in range(2):
            box(coll, f"{prefix}_VENT_T_{i}_{j}",
                (w / 2 - 30 * MM - i * 9 * MM, 40 * MM + j * 12 * MM, h / 2 + 0.2 * MM),
                (6 * MM, 7 * MM, 0.6 * MM), M["vent"], bevel=0.3 * MM)
    # 左侧散热长槽
    for i in range(6):
        box(coll, f"{prefix}_VENT_S_{i}",
            (-w / 2 - 0.2 * MM, -20 * MM + i * 16 * MM, 0),
            (0.6 * MM, 10 * MM, 2.2 * MM), M["vent"], bevel=0.2 * MM)
    # 后面板: 电源插座 + 风扇格栅 + 接地柱
    yr = d / 2 - 1.5 * MM
    box(coll, f"{prefix}_PSU", (w / 2 - 40 * MM, yr, 0), (60 * MM, 3 * MM, h - 10 * MM), M["plastic"], bevel=1.0 * MM)
    box(coll, f"{prefix}_PSU_in", (w / 2 - 40 * MM, yr + 1.6 * MM, 0), (24 * MM, 2 * MM, 16 * MM), M["cavity"])
    for i in range(2):
        cyl(coll, f"{prefix}_FAN_{i}", (-w / 2 + 35 * MM + i * 30 * MM, yr + 1.0 * MM, 0),
            11 * MM, 2.0 * MM, M["vent"], rot=(math.pi / 2, 0, 0))
        cyl(coll, f"{prefix}_FANH_{i}", (-w / 2 + 35 * MM + i * 30 * MM, yr + 1.2 * MM, 0),
            2.5 * MM, 2.4 * MM, M["fin"], rot=(math.pi / 2, 0, 0))
    cyl(coll, f"{prefix}_GND", (-w / 2 + 12 * MM, yr, -h / 2 + 8 * MM), 2.5 * MM, 3 * MM, M["cage"])
    # 品牌/型号丝印 (前面板左上)
    label_text(coll, f"{prefix}_LOGO", brand, (-w / 2 + 10 * MM, yf - 2.2 * MM, h / 2 - 7.5 * MM), 5.0, M["label"])
    label_text(coll, f"{prefix}_MODEL", model, (-w / 2 + 10 * MM, yf - 2.2 * MM, h / 2 - 14 * MM), 2.6, M["label"])
    # 前面板底部装饰条 (品牌色)
    box(coll, f"{prefix}_STRIP", (0, yf - 2.0 * MM, -h / 2 + 2.2 * MM),
        (w - 40 * MM, 0.8 * MM, 1.2 * MM), M["accent"], bevel=0.2 * MM)
    return yf - 2.0 * MM  # 端口安装面


def sys_leds(coll, prefix, x, y, z, M, labels=("PWR", "SYS", "STK")):
    """系统灯区 + 文字丝印, 返回 system leds 列表。"""
    out = []
    for i, lb in enumerate(labels):
        led = box(coll, f"{prefix}_LED_SYS_{lb}", (x + i * 8 * MM, y, z),
                  (2.6 * MM, 1.0 * MM, 2.0 * MM),
                  M["led_green"] if lb == "PWR" else M["led_off"], bevel=0.3 * MM, static=False)
        label_text(coll, f"{prefix}_LBL_{lb}", lb, (x + i * 8 * MM - 1.6 * MM, y, z - 3.4 * MM), 1.8, M["label"])
        out.append({"label": lb, "object": led.name,
                    "pos_mm": [round((x + i * 8 * MM) / MM, 2), round(y / MM, 2), round(z / MM, 2)]})
    return out


# ---------------------------------------------------------------- 装配辅助

def join_statics(coll, prefix, target_name):
    """把 collection 里 _static=True 的对象合并为单一网格(多材质)。"""
    statics = [o for o in coll.objects if o.type == "MESH" and o.get("_static")]
    if not statics:
        return None
    bpy.ops.object.select_all(action="DESELECT")
    for o in statics:
        o.select_set(True)
    bpy.context.view_layer.objects.active = statics[0]
    bpy.ops.object.join()
    act = bpy.context.active_object
    act.name = target_name
    return act


def export_fbx(coll, filepath):
    bpy.ops.object.select_all(action="DESELECT")
    for o in coll.objects:
        o.select_set(True)
    bpy.ops.export_scene.fbx(filepath=filepath, use_selection=True,
                             object_types={"MESH", "EMPTY"}, use_mesh_modifiers=True,
                             add_leaf_bones=False)
    bpy.ops.object.select_all(action="DESELECT")


def write_manifest(meta, filepath):
    with open(filepath, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=2)
