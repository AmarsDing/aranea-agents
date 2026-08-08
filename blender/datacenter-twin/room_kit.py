# -*- coding: utf-8 -*-
"""room_kit.py — 数据中心机房环境参数化建模
单位: 米。产出一个 ROOM Collection:
  - ROOM_Floor / ROOM_FloorSeams   (防静电地板 600x600 网格, 架空层)
  - ROOM_Wall_{N|S|E|W} / ROOM_Door
  - ROOM_Ceiling / ROOM_LightPanel_{rc}  (嵌入式灯盘, 自发光)
  - ROOM_Aisle_Cold / ROOM_Aisle_Hot     (冷/热通道地面标识带)
  - ROOM_ANCHOR_RACK_{id}                (Empty, 机柜排布吸附点)
坐标: front=-Y(冷通道), up=+Z, 地面完成面 z=0 (架空地板顶面)。
"""
import bpy, os, math, sys

sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import switch_kit as sk   # 复用材质/基础件

ROOT_DIR = os.path.dirname(os.path.abspath(__file__))
OUT_DIR = os.path.join(ROOT_DIR, 'models')

TILE = 0.600          # 防静电地板 600x600
FLOOR_VOID = 0.300    # 架空层高度
ROOM_W = 6.0          # X
ROOM_D = 6.0          # Y
ROOM_H = 3.0          # 吊顶净高


def init_materials():
    return {
        'floor':  sk._mat('M_FloorTile',  (0.62, 0.64, 0.66), 0.10, 0.50),
        'seam':   sk._mat('M_FloorSeam',  (0.30, 0.31, 0.33), 0.05, 0.70),
        'wall':   sk._mat('M_Wall',       (0.80, 0.82, 0.84), 0.00, 0.85),
        'ceil':   sk._mat('M_Ceiling',    (0.88, 0.89, 0.90), 0.00, 0.90),
        'door':   sk._mat('M_RoomDoor',   (0.35, 0.38, 0.42), 0.60, 0.45),
        'cold':   sk._mat('M_AisleCold',  (0.05, 0.25, 0.55), 0.10, 0.60),
        'hot':    sk._mat('M_AisleHot',   (0.55, 0.12, 0.08), 0.10, 0.60),
        'light':  sk._mat('M_LightPanel', (0.95, 0.96, 0.98), 0.00, 0.30,
                          (1.0, 1.0, 1.0), 8.0),
    }


def build_room(M=None, rack_anchors=None):
    """rack_anchors: [(name, x, y)] 机柜吸附点(前左角中心基准: 机柜中心)。"""
    if M is None:
        M = init_materials()
    coll = sk._coll('ROOM') if hasattr(sk, '_coll') else None
    if coll is None:
        coll = bpy.data.collections.get('ROOM') or bpy.data.collections.new('ROOM')
        bpy.context.scene.collection.children.link(coll)

    root = bpy.data.objects.get('ROOM_ROOT')
    if not root:
        root = bpy.data.objects.new('ROOM_ROOT', None)
        coll.objects.link(root)
    root['hw_category'] = 'ROOM'
    root['hw_name'] = 'Data Center Room Shell'
    root['hw_spec'] = '6x6m, raised floor 300mm, ceiling 3.0m, 600mm tile grid'

    nx = int(ROOM_W / TILE)
    ny = int(ROOM_D / TILE)
    w2, d2 = ROOM_W / 2, ROOM_D / 2

    # 地板整体板 + 缝网格 (缝 = 细长条, 减少对象数)
    sk.box(coll, 'ROOM_Floor', (0, 0, -0.01), (ROOM_W, ROOM_D, 0.02), M['floor'], bevel=0.0)
    seams = []
    for i in range(nx + 1):
        x = -w2 + i * TILE
        seams.append((0.004, ROOM_D, 0.002, x, 0, 0.001))
    for j in range(ny + 1):
        y = -d2 + j * TILE
        seams.append((ROOM_W, 0.004, 0.002, 0, y, 0.001))
    if hasattr(sk, 'multi_box'):
        sk.multi_box('ROOM_FloorSeams', seams, (0, 0, 0), M['seam'], None, coll)
    else:
        for k, s in enumerate(seams):
            sk.box(coll, 'ROOM_Seam_%d' % k, s[3:6], s[0:3], M['seam'])

    # 墙体 (南墙开 1.2m 双开门洞, 简化: 门扇直接贴上)
    t = 0.10
    sk.box(coll, 'ROOM_Wall_N', (0, d2 + t / 2, ROOM_H / 2), (ROOM_W + 2 * t, t, ROOM_H), M['wall'])
    sk.box(coll, 'ROOM_Wall_S', (0, -d2 - t / 2, ROOM_H / 2), (ROOM_W + 2 * t, t, ROOM_H), M['wall'])
    sk.box(coll, 'ROOM_Wall_E', (w2 + t / 2, 0, ROOM_H / 2), (t, ROOM_D, ROOM_H), M['wall'])
    sk.box(coll, 'ROOM_Wall_W', (-w2 - t / 2, 0, ROOM_H / 2), (t, ROOM_D, ROOM_H), M['wall'])
    sk.box(coll, 'ROOM_Door', (1.8, -d2 - t / 2 - 0.01, 1.05), (1.2, 0.05, 2.1), M['door'], bevel=0.01)

    # 吊顶 + 灯盘 (3 列 x 4 行, 1.2x0.3 面板)
    sk.box(coll, 'ROOM_Ceiling', (0, 0, ROOM_H + 0.01), (ROOM_W, ROOM_D, 0.02), M['ceil'])
    for r in range(4):
        for c in range(3):
            x = -1.8 + c * 1.8
            y = -2.25 + r * 1.5
            sk.box(coll, 'ROOM_LightPanel_%d_%d' % (r, c), (x, y, ROOM_H - 0.005),
                   (1.2, 0.3, 0.01), M['light'])

    # 冷/热通道标识带 (与 datacenter_layout.json aisles 对齐; 内沿避开机柜前后面 ±0.65)
    sk.box(coll, 'ROOM_Aisle_Cold', (0.2, -1.25, 0.003), (2.4, 1.2, 0.004), M['cold'])
    sk.box(coll, 'ROOM_Aisle_Hot', (0.2, 1.15, 0.003), (2.4, 1.0, 0.004), M['hot'])

    # 机柜吸附点
    for name, x, y in (rack_anchors or [('A01', 0.0, 0.0), ('A02', 0.7, 0.0)]):
        o = bpy.data.objects.get('ROOM_ANCHOR_RACK_' + name)
        if not o:
            o = bpy.data.objects.new('ROOM_ANCHOR_RACK_' + name, None)
            coll.objects.link(o)
        o.empty_display_size = 0.05
        o.location = (x, y, 0.0)
        o.parent = root
        o['hw_category'] = 'ANCHOR'
        o['rack_id'] = name
    return root
