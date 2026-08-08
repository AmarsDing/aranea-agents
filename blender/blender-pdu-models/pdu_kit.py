# -*- coding: utf-8 -*-
"""pdu_kit.py — 国产机柜 PDU 1:1 参数化建模工具库
单位: 米 (Blender 标准), 尺寸以毫米常量书写, 乘 MM 转换。
产出: 每台 PDU 一个 Collection, 含:
  - {PREFIX}_DETAIL                (静态合并网格: 机箱+插座+丝印)
  - {PREFIX}_LED_P{nn}             (每路插座独立 LED, UE5 动态材质实例改色)
  - {PREFIX}_LED_SYS_{label}       (系统灯: PWR/ALM/NET)
  - {PREFIX}_LCD                   (数据面板屏幕, 独立对象, UE5 RenderTarget/Widget 替换)
  - {PREFIX}_ANCHOR_P{nn}          (Empty, 插座 UI 挂点)
  - {PREFIX}_ANCHOR_LCD            (Empty, 大屏/详情 UI 挂点)
导出: FBX (MESH+EMPTY) + GLB + manifest JSON
约定: 前面板朝 -Y, 与交换机资产一致; three.js 侧 mapPos(mm)=(x, z, -y)。
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
        # 铝挤型材外壳: 银灰(公牛) / 阳极黑(突破/克莱沃)
        "alu_silver": _mat("M_AluSilver", (0.420, 0.440, 0.470), 0.85, 0.32),
        "alu_black":  _mat("M_AluBlack",  (0.045, 0.048, 0.055), 0.78, 0.38),
        "panel":      _mat("M_Panel",     (0.028, 0.030, 0.036), 0.45, 0.48),
        "panel_silver": _mat("M_PanelSilver", (0.360, 0.380, 0.410), 0.80, 0.36),
        "plastic":    _mat("M_Plastic",   (0.012, 0.013, 0.016), 0.10, 0.55),
        "socket":     _mat("M_Socket",    (0.020, 0.021, 0.025), 0.20, 0.42),   # 插座模块阻燃PC
        "socket_wh":  _mat("M_SocketWH",  (0.800, 0.810, 0.820), 0.10, 0.50),   # 浅色插座(可选)
        "cavity":     _mat("M_Cavity",    (0.003, 0.003, 0.004), 0.00, 1.00),   # 插孔保护门
        "red":        _mat("M_Red",       (0.520, 0.030, 0.035), 0.25, 0.40),   # 防脱扣/断路器
        "amber_sw":   _mat("M_AmberSw",   (0.850, 0.380, 0.020), 0.20, 0.42),   # 开关橙
        "cord":       _mat("M_Cord",      (0.010, 0.010, 0.012), 0.00, 0.85),   # 线缆橡胶
        "label":      _mat("M_Label",     (0.600, 0.630, 0.660), 0.30, 0.50),   # 丝印灰白
        "label_dark": _mat("M_LabelDark", (0.080, 0.085, 0.095), 0.20, 0.60),   # 深色丝印(银壳用)
        "accent":     _mat("M_Accent",    (0.020, 0.180, 0.300), 0.60, 0.30),
        # LCD: 默认幽暗青蓝(待机画面由 demo/UE5 覆盖)
        "lcd":        _mat("M_LCD",       (0.005, 0.020, 0.028), 0.20, 0.25, (0.00, 0.55, 0.70), 0.9),
        "lcd_bezel":  _mat("M_LcdBezel",  (0.008, 0.009, 0.011), 0.30, 0.35),
        # LED: 绿=正常 琥珀=高负载 红=告警/过载 灭=无输出
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


def box(coll, name, loc, size, mat, bevel=0.0, static=True, rot=(0, 0, 0)):
    bpy.ops.mesh.primitive_cube_add(location=loc, rotation=rot)
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


def label_text(coll, name, body, loc, size_mm, mat, static=True):
    cu = bpy.data.curves.new(name + "_cu", "FONT")
    cu.body = body
    cu.size = size_mm * MM
    cu.extrude = 0.12 * MM
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

# ---------------------------------------------------------------- PDU 机箱

def chassis(coll, prefix, w, d, h, M, brand, model, silver=False, rating=""):
    """铝挤 PDU 机身 + 前面板 + 挂耳 + 端盖 + 尾部进线 + 铭牌丝印。返回插座安装面 y。"""
    shell = M["alu_silver"] if silver else M["alu_black"]
    panel = M["panel_silver"] if silver else M["panel"]
    silk = M["label_dark"] if silver else M["label"]
    yf = -d / 2  # 前面板基准
    # 铝挤主身 (前留 3mm 面板位) + 顶/底挤型凹槽线 (特征筋条)
    box(coll, f"{prefix}_CHASSIS", (0, 1.5 * MM, 0), (w, d - 3 * MM, h), shell, bevel=1.6 * MM)
    for sz in (-1, 1):
        box(coll, f"{prefix}_RIB_T_{sz}", (0, 0.8 * MM, sz * (h / 2 - 1.2 * MM)),
            (w - 6 * MM, d - 8 * MM, 1.0 * MM), shell, bevel=0.3 * MM)
    # 前面板 (微凸 2mm)
    box(coll, f"{prefix}_PANEL", (0, yf - 1.0 * MM, 0), (w - 4 * MM, 2.0 * MM, h - 4 * MM), panel, bevel=1.0 * MM)
    # 两端塑料端盖
    for sx in (-1, 1):
        box(coll, f"{prefix}_CAP_{'L' if sx < 0 else 'R'}",
            (sx * (w / 2 - 1.5 * MM), 0, 0), (3 * MM, d - 2 * MM, h - 2 * MM), M["plastic"], bevel=1.0 * MM)
    # 19" 挂耳 + 安装孔
    for sx in (-1, 1):
        box(coll, f"{prefix}_EAR_{'L' if sx < 0 else 'R'}",
            (sx * (w / 2 + 9 * MM), yf + 1 * MM, 0), (18 * MM, 4 * MM, h), shell, bevel=0.8 * MM)
        for sz in (-1, 1):
            cyl(coll, f"{prefix}_EARH_{sx}_{sz}",
                (sx * (w / 2 + 9 * MM), yf - 1.2 * MM, sz * (h / 2 - 8 * MM)),
                2.4 * MM, 1.0 * MM, M["cavity"], rot=(math.pi / 2, 0, 0))
    # 尾部: 进线防水格兰头 + 线缆(向后再向下) + 接地柱
    yr = d / 2 - 1.5 * MM
    gx = -w / 2 + 22 * MM
    cyl(coll, f"{prefix}_GLAND", (gx, yr, 0), 7.5 * MM, 4 * MM, M["plastic"], rot=(math.pi / 2, 0, 0))
    cyl(coll, f"{prefix}_CORD1", (gx, yr + 16 * MM, 0), 4.2 * MM, 30 * MM, M["cord"], rot=(math.pi / 2, 0, 0))
    cyl(coll, f"{prefix}_CORD2", (gx, yr + 30 * MM, -14 * MM), 4.2 * MM, 28 * MM, M["cord"])
    cyl(coll, f"{prefix}_GND", (w / 2 - 14 * MM, yr, -h / 2 + 9 * MM), 2.5 * MM, 3 * MM, shell)
    # 品牌/型号/规格丝印 (左上品牌, 底缘型号/规格 — 避开系统灯与插座)
    label_text(coll, f"{prefix}_LOGO", brand, (-w / 2 + 10 * MM, yf - 2.2 * MM, h / 2 - 6.5 * MM), 4.6, silk)
    label_text(coll, f"{prefix}_MODEL", model, (-w / 2 + 10 * MM, yf - 2.2 * MM, -h / 2 + 3.7 * MM), 2.0, silk)
    if rating:
        label_text(coll, f"{prefix}_RATING", rating, (w / 2 - 96 * MM, yf - 2.2 * MM, -h / 2 + 3.7 * MM), 2.0, silk)
    return yf - 2.0 * MM  # 插座安装面

# ---------------------------------------------------------------- 新国标 10A 五孔插座模块

def outlet_gb10a(coll, prefix, idx, cx, cy, cz, M, led="green", anti_shed=False, wide=36.0):
    """GB 2099 新国标五孔模块: 斜二孔(左) + 品字三孔(右), 保护门深色, 前凸面板。
    (cx,cy,cz)=模块前面中心。led: 'green'|'amber'|'red'|'off'|None(无LED)。
    返回 outlet registry entry。"""
    Wm, Hm, D = wide * MM, 32.0 * MM, 10.0 * MM
    n = f"{prefix}_O{idx:02d}"
    # 模块壳体 + 前面框 + 内凹插孔面
    box(coll, n + "_housing", (cx, cy + D / 2 - 0.5 * MM, cz), (Wm, D, Hm), M["socket"], bevel=1.2 * MM)
    box(coll, n + "_frame", (cx, cy - 0.8 * MM, cz), (Wm - 1.5 * MM, 1.8 * MM, Hm - 1.5 * MM), M["socket"], bevel=0.8 * MM)
    face_y = cy - 1.9 * MM
    box(coll, n + "_face", (cx, face_y + 0.25 * MM, cz), (Wm - 4 * MM, 0.7 * MM, Hm - 4 * MM), M["plastic"], bevel=0.4 * MM)
    # --- 插孔 (保护门, 深灰近黑, 微凹) ---
    def hole(x, z, w, h, rot=0.0):
        box(coll, n + f"_hole_{x:.1f}_{z:.1f}", (cx + x * MM, face_y - 0.15 * MM, cz + z * MM),
            (w * MM, 0.6 * MM, h * MM), M["cavity"], bevel=0.3 * MM, rot=(0, rot, 0))
    # 斜二孔 (左侧, 新国标斜插)
    hole(-wide / 4 - 1.5, 3.2, 5.2, 1.8, rot=math.radians(28))
    hole(-wide / 4 - 1.5, -3.2, 5.2, 1.8, rot=math.radians(-28))
    # 品字三孔 (右侧): 地线竖孔在上, 两斜孔在下
    hole(wide / 4 + 1.5, 6.2, 2.0, 4.6)
    hole(wide / 4 - 1.2, -3.4, 5.0, 1.8, rot=math.radians(45))
    hole(wide / 4 + 4.2, -3.4, 5.0, 1.8, rot=math.radians(-45))
    # 防脱扣 (克莱沃特征): 模块顶部红色卡扣
    if anti_shed:
        box(coll, n + "_latch", (cx + Wm / 2 - 5.5 * MM, cy + 2.0 * MM, cz + Hm / 2 + 0.6 * MM),
            (7.0 * MM, 4.5 * MM, 2.6 * MM), M["red"], bevel=0.5 * MM)
    # LED (模块右上角, 独立对象) + 序号丝印
    led_name = None
    if led is not None:
        mat = {"green": M["led_green"], "amber": M["led_amber"], "red": M["led_red"]}.get(led, M["led_off"])
        lo = box(coll, f"{prefix}_LED_P{idx:02d}", (cx - Wm / 2 + 3.6 * MM, face_y - 0.4 * MM, cz + Hm / 2 - 3.2 * MM),
                 (2.6 * MM, 0.9 * MM, 2.0 * MM), mat, bevel=0.3 * MM, static=False)
        led_name = lo.name
    label_text(coll, n + "_num", "%02d" % idx,
               (cx + Wm / 2 - 7.4 * MM, face_y - 0.55 * MM, cz + Hm / 2 - 5.2 * MM), 2.0, M["label"])
    anc = anchor(coll, f"{prefix}_ANCHOR_P{idx:02d}", (cx, cy - 14 * MM, cz + Hm / 2 + 10 * MM))
    return {"index": idx, "type": "GB10A", "led": led_name, "anchor": anc.name,
            "pos_mm": [round(cx / MM, 2), round(cy / MM, 2), round(cz / MM, 2)]}

# ---------------------------------------------------------------- LCD 数据面板

def lcd_module(coll, prefix, cx, cy, cz, M, w=42.0, h=26.0):
    """嵌入式 LCD 计量表: 边框 + 玻璃 + 独立发光屏对象 {PREFIX}_LCD。
    demo/UE5 用该对象名挂 CanvasTexture / RenderTarget。返回 lcd entry。"""
    n = f"{prefix}_LCDM"
    box(coll, n + "_bezel", (cx, cy + 1.0 * MM, cz), (w * MM, 4.0 * MM, h * MM), M["lcd_bezel"], bevel=1.0 * MM)
    # 玻璃面 (微凸, 深色)
    box(coll, n + "_glass", (cx, cy - 1.15 * MM, cz), ((w - 3) * MM, 0.8 * MM, (h - 3) * MM), M["cavity"], bevel=0.5 * MM)
    # 发光屏: 独立平面 (非 box) —— cube 默认 UV 前面只占 1/4 纹理, CanvasTexture/RenderTarget 会错位;
    # plane UV 天然全幅 0..1, 正视 U 向右 V 向上, GLTF 翻转后与图像坐标一致。
    bpy.ops.mesh.primitive_plane_add(size=1, location=(cx, cy - 1.7 * MM, cz), rotation=(math.radians(90), 0, 0))
    scr = bpy.context.active_object
    scr.name = f"{prefix}_LCD"
    scr.scale = ((w - 6) * MM / 2, (h - 6) * MM / 2, 1)
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    scr.data.materials.append(M["lcd"])
    scr["_static"] = False
    _link(coll, scr)
    anc = anchor(coll, f"{prefix}_ANCHOR_LCD", (cx, cy - 30 * MM, cz + h / 2 * MM + 12 * MM))
    return {"object": scr.name, "anchor": anc.name,
            "pos_mm": [round(cx / MM, 2), round((cy - 1.7 * MM) / MM, 2), round(cz / MM, 2)],
            "size_mm": [w - 6, h - 6]}

# ---------------------------------------------------------------- 保护/通讯件

def rocker_breaker(coll, prefix, cx, cy, cz, M, label="过载保护"):
    """跷板式过载保护开关 (公牛式): 黑色基座 + 红色跷板 + ALM 灯。返回 sys led entry。"""
    n = f"{prefix}_BRK"
    box(coll, n + "_base", (cx, cy + 1.5 * MM, cz), (15 * MM, 5 * MM, 22 * MM), M["plastic"], bevel=0.8 * MM)
    box(coll, n + "_rock", (cx, cy - 1.6 * MM, cz + 1.5 * MM), (11 * MM, 2.2 * MM, 15 * MM), M["red"],
        bevel=0.6 * MM, rot=(math.radians(-8), 0, 0))
    led = box(coll, f"{prefix}_LED_SYS_ALM", (cx, cy - 2.0 * MM, cz - 8.6 * MM),
              (2.8 * MM, 0.9 * MM, 2.2 * MM), M["led_off"], bevel=0.3 * MM, static=False)
    return {"label": "ALM", "object": led.name,
            "pos_mm": [round(cx / MM, 2), round((cy - 2.0 * MM) / MM, 2), round((cz - 8.6 * MM) / MM, 2)]}


def hydraulic_breaker(coll, prefix, cx, cy, cz, M, amps=32):
    """液压电磁断路器 (克莱沃式): 横向圆柱手柄 + 基座 + 电流丝印 + ALM 灯。"""
    n = f"{prefix}_BRK"
    box(coll, n + "_base", (cx, cy + 2 * MM, cz), (20 * MM, 6 * MM, 30 * MM), M["plastic"], bevel=0.8 * MM)
    cyl(coll, n + "_handle", (cx, cy - 2.2 * MM, cz + 2 * MM), 4.2 * MM, 14 * MM, M["plastic"],
        rot=(math.radians(90), 0, 0))
    label_text(coll, n + "_amps", "%dA" % amps, (cx - 6 * MM, cy - 3.4 * MM, cz - 9.5 * MM), 2.4, M["label"])
    led = box(coll, f"{prefix}_LED_SYS_ALM", (cx + 7 * MM, cy - 2.0 * MM, cz - 9.5 * MM),
              (2.8 * MM, 0.9 * MM, 2.2 * MM), M["led_off"], bevel=0.3 * MM, static=False)
    return {"label": "ALM", "object": led.name,
            "pos_mm": [round((cx + 7 * MM) / MM, 2), round((cy - 2.0 * MM) / MM, 2), round((cz - 9.5 * MM) / MM, 2)]}


def rj45_net(coll, prefix, cx, cy, cz, M, name="NET"):
    """网口 (智能 PDU 通讯): 简化 RJ45, 静态。"""
    W, H, D = 15.0 * MM, 13.2 * MM, 10.0 * MM
    n = f"{prefix}_{name}"
    box(coll, n + "_housing", (cx, cy + D / 2 - 0.5 * MM, cz), (W, D, H), M["plastic"], bevel=0.6 * MM)
    box(coll, n + "_cavity", (cx, cy - 0.6 * MM, cz + 0.4 * MM), (W - 1.6 * MM, 1.6 * MM, H - 2.4 * MM), M["cavity"])
    box(coll, n + "_gold", (cx, cy - 1.2 * MM, cz + H / 2 - 2.2 * MM), (W - 4.2 * MM, 0.5 * MM, 1.6 * MM),
        M["label"])
    label_text(coll, n + "_lbl", name, (cx - 6.5 * MM, cy - 1.0 * MM, cz - H / 2 - 3.2 * MM), 1.8, M["label"])


def sys_leds(coll, prefix, x, y, z, M, labels=("PWR", "NET"), states=None):
    """系统灯区 (无丝印标签, 避免拥挤), 返回 system leds 列表。states: {label:'green'|'blue'|'off'}"""
    out = []
    states = states or {}
    for i, lb in enumerate(labels):
        st = states.get(lb, "green" if lb == "PWR" else "off")
        mat = {"green": M["led_green"], "blue": M["led_blue"], "red": M["led_red"]}.get(st, M["led_off"])
        led = box(coll, f"{prefix}_LED_SYS_{lb}", (x + i * 8 * MM, y, z),
                  (2.6 * MM, 1.0 * MM, 2.0 * MM), mat, bevel=0.3 * MM, static=False)
        out.append({"label": lb, "object": led.name,
                    "pos_mm": [round((x + i * 8 * MM) / MM, 2), round(y / MM, 2), round(z / MM, 2)]})
    return out

# ---------------------------------------------------------------- 装配辅助

def join_statics(coll, prefix, target_name):
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


def write_manifest(meta, filepath):
    with open(filepath, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=2)
