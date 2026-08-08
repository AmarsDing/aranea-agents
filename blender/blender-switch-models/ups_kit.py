# -*- coding: utf-8 -*-
"""ups_kit.py — 国产主流 UPS 1:1 参数化建模工具库
单位约定: 对外函数全部使用毫米(mm), 内部乘 MM 转米; 塔式原点=底面中心(z=0 落地), 前面朝 -Y。
产出(每台 UPS 一个 Collection):
  - {P}_DETAIL            静态合并网格(机箱/面板/插座/风扇/丝印)
  - {P}_SCREEN            LCD 发光屏(独立 Mesh, UE5 替换为动态 UI 材质/WidgetComponent)
  - {P}_LED_{MAINS|BYPASS|INVERT|BATTERY|FAULT}   状态灯(独立 Mesh, 动态材质实例改色)
  - {P}_LED_LOAD_{1..5}   输出负载段显(输出电流水平)
  - {P}_LED_BATT_{1..5}   电池电量段显
  - {P}_BTN_* / 其他      静态
  - {P}_ANCHOR_{PANEL|SCREEN|INPUT|OUTPUT|BATT}   Empty, UE5 UI 挂点
基础件(box/cyl/anchor/text/join)复用 switch_kit, 避免重复造轮子。
"""
import bpy
import math
import os
import switch_kit as sk

MM = sk.MM
ROOT = sk.ROOT
OUT_DIR = sk.OUT_DIR

box = sk.box
cyl = sk.cyl
anchor = sk.anchor
join_statics = sk.join_statics
write_manifest = sk.write_manifest


# ---------------------------------------------------------------- 材质

def init_materials():
    M = sk.init_materials()  # 基础: chassis/panel/plastic/cavity/gold/cage/fin/vent/label/accent/led_*
    M.update({
        # UPS 机身配色: 深灰金属机身 + 磨砂黑面板 + 品牌点缀
        "ups_body":   sk._mat("M_UPS_BODY",   (0.048, 0.052, 0.060), 0.55, 0.45),
        "ups_bezel":  sk._mat("M_UPS_BEZEL",  (0.034, 0.037, 0.044), 0.35, 0.52),
        "ups_rear":   sk._mat("M_UPS_REAR",   (0.042, 0.045, 0.052), 0.60, 0.48),
        "socket":     sk._mat("M_SOCKET",     (0.020, 0.021, 0.026), 0.15, 0.55),
        "breaker":    sk._mat("M_BREAKER",    (0.030, 0.032, 0.038), 0.20, 0.50),
        "breaker_on": sk._mat("M_BREAKER_ON", (0.500, 0.020, 0.030), 0.10, 0.45),
        "copper":     sk._mat("M_COPPER",     (0.720, 0.400, 0.180), 1.00, 0.25),
        "term_red":   sk._mat("M_TERM_RED",   (0.620, 0.040, 0.050), 0.30, 0.40),
        "term_blk":   sk._mat("M_TERM_BLK",   (0.015, 0.016, 0.020), 0.30, 0.40),
        "wheel":      sk._mat("M_WHEEL",      (0.012, 0.012, 0.015), 0.05, 0.85),
        "btn":        sk._mat("M_BTN",        (0.160, 0.170, 0.190), 0.85, 0.30),
        # LCD: 深蓝背光屏 + 青色微发光, 科技感载体
        "lcd":        sk._mat("M_LCD",        (0.006, 0.022, 0.040), 0.10, 0.25,
                              (0.05, 0.30, 0.50), 1.2),
        "lcd_bezel":  sk._mat("M_LCD_BEZEL",  (0.012, 0.013, 0.017), 0.30, 0.35),
        # 白色丝印(UPS 面板常用)
        "silk":       sk._mat("M_SILK",       (0.780, 0.810, 0.840), 0.10, 0.55),
        "silk_dim":   sk._mat("M_SILK_DIM",   (0.380, 0.410, 0.450), 0.10, 0.60),
    })
    return M


# ---------------------------------------------------------------- 文字(可指定朝向)

def text(coll, name, body, loc, size_mm, mat, face="front", align="LEFT", extrude=0.10 * MM):
    """face: front=朝 -Y(前面板) / rear=朝 +Y(后面板) / top=躺平朝上(+Z)。"""
    o = sk.label_text(coll, name, body, loc, size_mm, mat, extrude=extrude)
    if face == "front":
        o.rotation_euler = (math.pi / 2, 0, 0)
    elif face == "rear":
        o.rotation_euler = (math.pi / 2, 0, math.pi)
    elif face == "top":
        o.rotation_euler = (0, 0, 0)
    if align == "CENTER":
        # 已转网格, 用原点偏移实现居中: 重新设质心近似即可——简单方案: 按字符数估算宽度
        w_est = len(body) * size_mm * 0.62 * MM
        if face == "front":
            o.location.x -= w_est / 2
        elif face == "rear":
            o.location.x += w_est / 2
        else:
            o.location.x -= w_est / 2
    return o


# ---------------------------------------------------------------- 机身

def tower_body(coll, P, W, D, H, M, brand, model, sub, base_h=8.0):
    """塔式机身。W/D/H mm(不含底座), base_h=支脚/脚轮腾空高度 mm。
    返回 (fy, ry): 前面安装面 y / 后面安装面 y (米)。"""
    Wm, Dm, Hm, Bm = W * MM, D * MM, H * MM, base_h * MM
    zc = Bm + Hm / 2
    # 主箱体(前后各留 3mm 给面板)
    box(coll, P + "_CHASSIS", (0, 0, zc), (Wm - 4 * MM, Dm - 6 * MM, Hm), M["ups_body"], bevel=5 * MM)
    # 前面板装饰罩(上部 55% 高度, 微凸)
    box(coll, P + "_BEZEL", (0, -Dm / 2 - 1.6 * MM, Bm + Hm * 0.725),
        (Wm - 12 * MM, 3.2 * MM, Hm * 0.53), M["ups_bezel"], bevel=2.5 * MM)
    # 前面下部进风栅(横槽)
    nv = 9
    for i in range(nv):
        z = Bm + Hm * 0.075 + i * (Hm * 0.33 / nv)
        box(coll, f"{P}_VENT_F_{i}", (0, -Dm / 2 - 0.4 * MM, z),
            (Wm - 60 * MM, 1.2 * MM, 2.6 * MM), M["vent"], bevel=0.5 * MM)
    # 侧面出风槽(两侧后部)
    for sx in (-1, 1):
        for i in range(6):
            box(coll, f"{P}_VENT_S_{sx}_{i}",
                (sx * (Wm / 2 - 0.2 * MM), Dm * 0.18 + i * 14 * MM - 42 * MM, Bm + Hm * 0.30),
                (1.0 * MM, 9 * MM, 3.0 * MM), M["vent"], bevel=0.3 * MM)
    # 顶盖接缝线
    box(coll, P + "_LID", (0, 0, Bm + Hm - 1.0 * MM), (Wm - 6 * MM, Dm - 8 * MM, 2.0 * MM),
        M["ups_bezel"], bevel=1.0 * MM)
    # 后面板(微凸 1.5mm)
    box(coll, P + "_REAR", (0, Dm / 2 + 0.8 * MM, zc), (Wm - 10 * MM, 2.0 * MM, Hm - 12 * MM),
        M["ups_rear"], bevel=1.0 * MM)
    # 品牌丝印(前面板顶区)
    text(coll, P + "_LOGO", brand, (-Wm / 2 + 14 * MM, -Dm / 2 - 3.6 * MM, Bm + Hm - 22 * MM),
         9.0, M["silk"])
    text(coll, P + "_MODEL", model, (-Wm / 2 + 14 * MM, -Dm / 2 - 3.6 * MM, Bm + Hm - 33 * MM),
         3.4, M["silk_dim"])
    if sub:
        text(coll, P + "_SUB", sub, (-Wm / 2 + 14 * MM, -Dm / 2 - 3.6 * MM, Bm + Hm - 41 * MM),
             2.4, M["silk_dim"])
    return -Dm / 2 - 3.2 * MM, Dm / 2 + 1.8 * MM


def feet(coll, P, W, D, M, h=8.0, r=9.0):
    """塔式橡胶支脚×4, 底部 z=0。"""
    for sx in (-1, 1):
        for sy in (-1, 1):
            cyl(coll, f"{P}_FOOT_{sx}_{sy}", (sx * (W / 2 - 26) * MM, sy * (D / 2 - 26) * MM, h / 2 * MM),
                r * MM, h * MM, M["wheel"], verts=20)


def wheels(coll, P, W, D, M, h=70.0):
    """脚轮×4(万向轮: 支架+轮), 底部 z=0, 轮半径≈h*0.36。"""
    r = h * 0.36
    for sx in (-1, 1):
        for sy in (-1, 1):
            x, y = sx * (W / 2 - 30) * MM, sy * (D / 2 - 34) * MM
            box(coll, f"{P}_CASTER_{sx}_{sy}", (x, y, (h - 6) * MM), (16 * MM, 16 * MM, 12 * MM),
                M["ups_bezel"], bevel=2 * MM)
            cyl(coll, f"{P}_WHEEL_{sx}_{sy}", (x, y, r * MM), r * MM, 12 * MM, M["wheel"],
                rot=(0, math.pi / 2, 0), verts=20)


def rack_body(coll, P, W, D, H, M, brand, model):
    """机架式机身(1U/2U)。原点=底面中心, 前面朝 -Y。返回 (fy, ry)。"""
    Wm, Dm, Hm = W * MM, D * MM, H * MM
    zc = Hm / 2
    box(coll, P + "_CHASSIS", (0, 0, zc), (Wm, Dm - 4 * MM, Hm), M["ups_body"], bevel=2.0 * MM)
    # 前面板(整面微凸)
    box(coll, P + "_PANEL", (0, -Dm / 2 - 1.2 * MM, zc), (Wm - 6 * MM, 2.4 * MM, Hm - 6 * MM),
        M["ups_bezel"], bevel=1.2 * MM)
    # 挂耳(19 英寸)
    for sx in (-1, 1):
        box(coll, f"{P}_EAR_{sx}", (sx * (Wm / 2 + 12 * MM), -Dm / 2 + 1 * MM, zc),
            (24 * MM, 4 * MM, Hm), M["ups_body"], bevel=0.8 * MM)
        for sz in (-1, 1):
            cyl(coll, f"{P}_EARH_{sx}_{sz}",
                (sx * (Wm / 2 + 12 * MM), -Dm / 2 - 1.4 * MM, zc + sz * (Hm / 2 - 12 * MM)),
                2.6 * MM, 1.2 * MM, M["cavity"], rot=(math.pi / 2, 0, 0))
    # 左部进风孔阵(6 列 x 2 排细长孔)
    for i in range(6):
        for j in range(2):
            box(coll, f"{P}_VENT_F_{i}_{j}",
                (-Wm / 2 + 26 * MM + i * 11 * MM, -Dm / 2 - 2.6 * MM, zc + (j - 0.5) * 14 * MM),
                (7 * MM, 1.0 * MM, 3.2 * MM), M["vent"], bevel=0.4 * MM)
    # 顶部散热孔阵(右后区)
    for i in range(10):
        for j in range(3):
            box(coll, f"{P}_VENT_T_{i}_{j}",
                (Wm / 2 - 26 * MM - i * 10 * MM, Dm * 0.22 + j * 13 * MM - 40 * MM, Hm + 0.2 * MM),
                (6 * MM, 8 * MM, 0.6 * MM), M["vent"], bevel=0.3 * MM)
    # 后面板
    box(coll, P + "_REAR", (0, Dm / 2 + 0.8 * MM, zc), (Wm - 8 * MM, 2.0 * MM, Hm - 8 * MM),
        M["ups_rear"], bevel=0.8 * MM)
    # 品牌丝印
    text(coll, P + "_LOGO", brand, (-Wm / 2 + 10 * MM, -Dm / 2 - 2.8 * MM, Hm - 12 * MM),
         5.0, M["silk"])
    text(coll, P + "_MODEL", model, (-Wm / 2 + 10 * MM, -Dm / 2 - 2.8 * MM, Hm - 19 * MM),
         2.4, M["silk_dim"])
    return -Dm / 2 - 2.4 * MM, Dm / 2 + 1.8 * MM


# ---------------------------------------------------------------- 面板件

def lcd(coll, P, x, y, z, w, h, M, anchor_dy=-26.0):
    """LCD 屏: 边框 + 发光屏(独立对象 {P}_SCREEN)。返回 screen 记录。"""
    box(coll, P + "_LCD bezel", (x, y + 1.2 * MM, z), (w + 8 * MM, 5 * MM, h + 8 * MM),
        M["lcd_bezel"], bevel=2.0 * MM)
    scr = box(coll, P + "_SCREEN", (x, y - 1.6 * MM, z), (w, 1.2 * MM, h), M["lcd"],
              bevel=1.0 * MM, static=False)
    anc = anchor(coll, P + "_ANCHOR_SCREEN", (x, y + anchor_dy * MM, z + h / 2 + 12 * MM))
    return {"object": scr.name, "anchor": anc.name,
            "pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)],
            "size_mm": [round(w / MM, 1), round(h / MM, 1)]}


def status_led(coll, P, label, x, y, z, M, color="off", text_mat=None):
    """状态灯(圆角小方块) + 下方丝印标签。color: green/amber/red/off。"""
    led = box(coll, f"{P}_LED_{label}", (x, y, z), (4.6 * MM, 1.2 * MM, 3.0 * MM),
              M["led_" + color], bevel=0.5 * MM, static=False)
    tm = text_mat or M["silk_dim"]
    text(coll, f"{P}_LBL_{label}", label, (x - 4.2 * MM, y - 0.2 * MM, z - 6.2 * MM), 1.9, tm)
    return {"label": label, "object": led.name,
            "pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)]}


def seg_bar(coll, P, kind, x, y, z, M, lit, lit_color, n=5, title=None):
    """段显条(横向): kind=LOAD/BATT, lit=点亮段数, lit_color=green/amber/red。
    每段 7.5w x 3.6h mm。返回 led 记录列表。"""
    out = []
    sw, sh, gap = 7.5 * MM, 3.6 * MM, 2.4 * MM
    total = n * sw + (n - 1) * gap
    # 底槽
    box(coll, f"{P}_{kind}_SLOT", (x, y + 0.8 * MM, z), (total + 5 * MM, 2.0 * MM, sh + 4 * MM),
        M["cavity"], bevel=1.0 * MM)
    for i in range(n):
        sx = x - total / 2 + sw / 2 + i * (sw + gap)
        on = i < lit
        mat = M["led_" + lit_color] if on else M["led_off"]
        led = box(coll, f"{P}_LED_{kind}_{i + 1}", (sx, y - 0.4 * MM, z), (sw, 1.2 * MM, sh),
                  mat, bevel=0.4 * MM, static=False)
        out.append({"label": f"{kind}_{i + 1}", "object": led.name,
                    "pos_mm": [round(sx / MM, 2), round(y / MM, 2), round(z / MM, 2)]})
    if title:
        text(coll, f"{P}_LBL_{kind}", title, (x - total / 2, y - 0.2 * MM, z + 7.0 * MM), 2.0,
             M["silk_dim"])
    return out


def power_button(coll, P, x, y, z, M, r=8.0, label=None):
    """金属圈电源键。"""
    cyl(coll, P + "_BTN_RING", (x, y + 0.6 * MM, z), (r + 1.6) * MM, 2.0 * MM, M["btn"],
        rot=(math.pi / 2, 0, 0), verts=28)
    cyl(coll, P + "_BTN", (x, y - 0.6 * MM, z), r * MM, 2.2 * MM, M["ups_bezel"],
        rot=(math.pi / 2, 0, 0), verts=28)
    # 电源符号(小竖条+弧近似)
    box(coll, P + "_BTN_SYM", (x, y - 1.8 * MM, z + r * 0.30 * MM),
        (1.0 * MM, 0.6 * MM, r * 0.75 * MM), M["silk"])
    if label:
        text(coll, P + "_LBL_PWR", label, (x - 3.5 * MM, y - 0.2 * MM, z - r * MM - 5 * MM),
             2.0, M["silk_dim"])


def panel_button(coll, P, name, x, y, z, M, w=10.0, h=5.0, label=None):
    box(coll, f"{P}_BTN_{name}", (x, y, z), (w * MM, 1.6 * MM, h * MM), M["btn"], bevel=0.8 * MM)
    if label:
        text(coll, f"{P}_LBL_{name}", label, (x - w / 2 * MM, y - 0.2 * MM, z - 6.4 * MM),
             1.8, M["silk_dim"])


# ---------------------------------------------------------------- 后面板件

def cn_socket(coll, P, idx, x, y, z, M):
    """国标 10A 五孔插座(面板 34x34, 朝 +Y 后方)。"""
    n = f"{P}_OUT{idx:02d}"
    box(coll, n + "_plate", (x, y, z), (34 * MM, 3 * MM, 34 * MM), M["socket"], bevel=1.5 * MM)
    # 二孔(上, 斜八字近似两竖孔)
    for sx in (-1, 1):
        box(coll, f"{n}_h2_{sx}", (x + sx * 4.2 * MM, y + 1.6 * MM, z + 6.5 * MM),
            (2.2 * MM, 1.2 * MM, 6.0 * MM), M["cavity"], bevel=0.3 * MM)
    # 三孔(下, 品字)
    box(coll, n + "_h3g", (x, y + 1.6 * MM, z - 4.0 * MM), (2.6 * MM, 1.2 * MM, 5.6 * MM), M["cavity"])
    for sx in (-1, 1):
        box(coll, f"{n}_h3_{sx}", (x + sx * 4.6 * MM, y + 1.6 * MM, z - 8.2 * MM),
            (2.2 * MM, 1.2 * MM, 5.2 * MM), M["cavity"], bevel=0.3 * MM)
    return {"index": idx, "object": None, "pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)]}


def breaker(coll, P, x, y, z, M, label="INPUT"):
    """输入空开(摇臂开关, 红色 ON 侧)。"""
    box(coll, P + "_BRK", (x, y, z), (26 * MM, 6 * MM, 30 * MM), M["breaker"], bevel=1.2 * MM)
    box(coll, P + "_BRK_SW", (x, y + 3.2 * MM, z + 6 * MM), (14 * MM, 4 * MM, 10 * MM),
        M["breaker_on"], bevel=1.0 * MM)
    text(coll, P + "_LBL_BRK", label, (x - 6 * MM, y + 3.4 * MM, z - 19 * MM), 2.2, M["silk"],
         face="rear")
    return {"pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)]}


def terminal_cover(coll, P, name, x, y, z, w, h, M, label=None):
    """接线端子盖板(输入/输出/电池), 带两颗手拧螺丝。"""
    box(coll, f"{P}_{name}", (x, y, z), (w * MM, 4 * MM, h * MM), M["ups_rear"], bevel=1.0 * MM)
    for sx in (-1, 1):
        cyl(coll, f"{P}_{name}_SCR_{sx}", (x + sx * (w / 2 - 6) * MM, y + 2.2 * MM, z),
            2.2 * MM, 1.6 * MM, M["cage"], rot=(math.pi / 2, 0, 0), verts=16)
    if label:
        text(coll, f"{P}_LBL_{name}", label, (x - w / 2 * MM + 3 * MM, y + 2.4 * MM, z - 5 * MM),
             2.2, M["silk"], face="rear")
    return {"pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)]}


def batt_terminals(coll, P, x, y, z, M):
    """外接电池大电流接线柱(红正黑负)。"""
    for i, (sx, key) in enumerate(((-1, "term_red"), (1, "term_blk"))):
        tx = x + sx * 14 * MM
        cyl(coll, f"{P}_BATTP_{i}", (tx, y, z), 7 * MM, 5 * MM, M[key],
            rot=(math.pi / 2, 0, 0), verts=20)
        cyl(coll, f"{P}_BATTC_{i}", (tx, y + 2.8 * MM, z), 3 * MM, 2 * MM, M["copper"],
            rot=(math.pi / 2, 0, 0), verts=16)
    text(coll, P + "_LBL_BATT", "BAT 192VDC", (x - 13 * MM, y + 2.6 * MM, z - 13 * MM),
         2.2, M["silk"], face="rear")
    return {"pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)]}


def fan(coll, P, idx, x, y, z, M, r=38.0):
    """散热风扇(外圈+轮毂+叶片示意)。"""
    n = f"{P}_FAN{idx}"
    cyl(coll, n + "_ring", (x, y, z), r * MM, 3 * MM, M["vent"], rot=(math.pi / 2, 0, 0), verts=32)
    cyl(coll, n + "_hub", (x, y + 1.6 * MM, z), r * 0.36 * MM, 3.4 * MM, M["ups_bezel"],
        rot=(math.pi / 2, 0, 0), verts=20)
    for k in range(7):
        a = k * math.tau / 7
        bx, bz = x + math.cos(a) * r * 0.62 * MM, z + math.sin(a) * r * 0.62 * MM
        box(coll, f"{n}_bl_{k}", (bx, y + 1.4 * MM, bz),
            (r * 0.52 * MM, 1.0 * MM, r * 0.20 * MM), M["vent"], bevel=0.4 * MM)
    return {"pos_mm": [round(x / MM, 2), round(y / MM, 2), round(z / MM, 2)]}


def comm_db9(coll, P, name, x, y, z, M, label="RS232"):
    """DB9 串口。"""
    box(coll, f"{P}_{name}", (x, y, z), (20 * MM, 4 * MM, 10 * MM), M["cage"], bevel=0.8 * MM)
    box(coll, f"{P}_{name}_in", (x, y + 2.2 * MM, z), (16 * MM, 1.6 * MM, 6 * MM), M["cavity"])
    if label:
        text(coll, f"{P}_LBL_{name}", label, (x - 7 * MM, y + 2.4 * MM, z - 9 * MM), 1.8,
             M["silk_dim"], face="rear")


def comm_usb(coll, P, name, x, y, z, M, label="USB"):
    box(coll, f"{P}_{name}", (x, y, z), (12 * MM, 4 * MM, 11 * MM), M["cage"], bevel=0.6 * MM)
    box(coll, f"{P}_{name}_in", (x, y + 2.2 * MM, z), (9 * MM, 1.6 * MM, 8 * MM), M["cavity"])
    if label:
        text(coll, f"{P}_LBL_{name}", label, (x - 4 * MM, y + 2.4 * MM, z - 9 * MM), 1.8,
             M["silk_dim"], face="rear")


def comm_rj45(coll, P, name, x, y, z, M, label="EPO"):
    """通信 RJ45(EPO/RS485/并机), 无 LED。"""
    box(coll, f"{P}_{name}", (x, y, z), (15 * MM, 4 * MM, 13.2 * MM), M["plastic"], bevel=0.6 * MM)
    box(coll, f"{P}_{name}_in", (x, y + 2.2 * MM, z + 0.4 * MM), (13 * MM, 1.6 * MM, 10.6 * MM),
        M["cavity"])
    box(coll, f"{P}_{name}_gold", (x, y + 2.4 * MM, z + 4.2 * MM), (10 * MM, 1.0 * MM, 1.6 * MM),
        M["gold"])
    if label:
        text(coll, f"{P}_LBL_{name}", label, (x - 4 * MM, y + 2.4 * MM, z - 10 * MM), 1.8,
             M["silk_dim"], face="rear")


def slot_cover(coll, P, name, x, y, z, w, h, M, label="SNMP"):
    """智能插槽盖板(SNMP/干接点卡槽)。"""
    box(coll, f"{P}_{name}", (x, y, z), (w * MM, 2.5 * MM, h * MM), M["ups_bezel"], bevel=1.0 * MM)
    for sx in (-1, 1):
        cyl(coll, f"{P}_{name}_SCR_{sx}", (x + sx * (w / 2 - 5) * MM, y + 1.6 * MM, z),
            2.0 * MM, 1.4 * MM, M["cage"], rot=(math.pi / 2, 0, 0), verts=14)
    if label:
        text(coll, f"{P}_LBL_{name}", label, (x - 8 * MM, y + 2.0 * MM, z - 4 * MM), 2.0,
             M["silk_dim"], face="rear")


def gnd_post(coll, P, x, y, z, M):
    cyl(coll, P + "_GND", (x, y, z), 3 * MM, 4 * MM, M["copper"], rot=(math.pi / 2, 0, 0))
    text(coll, P + "_LBL_GND", "PE", (x - 2.6 * MM, y + 2.2 * MM, z - 8 * MM), 1.8,
         M["silk_dim"], face="rear")
