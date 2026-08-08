# -*- coding: utf-8 -*-
"""rerender_showcase.py — 仅重渲 ups_showcase.png, 不重建模型/不重导出
用法: blender -b models/ups_showcase.blend --python rerender_showcase.py
改动: 三机弧形布局(华为抬高上机架底座) + 低机位平视 + 灯光提亮。
"""
import sys, os, math
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy

ROOT = os.path.dirname(os.path.abspath(__file__))


def find(n):
    return bpy.data.objects.get(n)


def layout():
    # UPS1 山特 (左塔) / UPS2 华为 (中间, 上机架底座) / UPS3 科士达 (右塔)
    r1 = find('ROOT_UPS1_C6KS')
    r2 = find('ROOT_UPS2_HU10K')
    r3 = find('ROOT_UPS3_YDC10H')
    if r1:
        r1.location = (-0.56, 0.10, 0.0)
        r1.rotation_euler = (0, 0, math.radians(14))
    if r3:
        r3.location = (0.58, 0.12, 0.0)
        r3.rotation_euler = (0, 0, math.radians(-14))
    # 机架底座 (华为 2U 垫高, 深色钢柜)
    base = find('UPS_RACK_BASE')
    if not base:
        bpy.ops.mesh.primitive_cube_add(location=(0.0, -0.30, 0.12))
        base = bpy.context.active_object
        base.name = 'UPS_RACK_BASE'
        base.scale = (0.27, 0.34, 0.12)
        bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
        m = bpy.data.materials.get('M_UPS_BODY')
        if m:
            base.data.materials.append(m)
        mod = base.modifiers.new('Bev', 'BEVEL')
        mod.width = 0.008
        mod.segments = 2
        bpy.ops.object.modifier_apply(modifier=mod.name)
    if r2:
        r2.location = (0.0, -0.32, 0.245)
        r2.rotation_euler = (0, 0, 0)
    bpy.context.view_layer.update()


def relight():
    for n, e in (('UPSKey', 16), ('UPSFill', 10), ('UPSRim', 18),
                 ('UPSTop', 6), ('UPSFront', 14)):
        o = find(n)
        if o and o.type == 'LIGHT':
            o.data.energy = e
    acc = find('UPSAccent')
    if acc and acc.type == 'LIGHT':
        acc.data.energy = 4.0


def main():
    layout()
    relight()
    cam = find('UPSCam')
    tgt = find('UPSTarget')
    cam.location = (0.0, -2.45, 0.66)
    tgt.location = (0.0, -0.06, 0.26)
    cam.data.lens = 47
    sc = bpy.context.scene
    sc.render.resolution_x, sc.render.resolution_y = 1920, 1080
    sc.render.resolution_percentage = 100
    sc.render.filepath = os.path.join(ROOT, 'ups_showcase.png')
    bpy.context.view_layer.update()
    bpy.ops.render.render(write_still=True)
    print('[showcase] rendered')
    bpy.ops.wm.save_as_mainfile(filepath=os.path.join(
        ROOT, 'models', 'ups_showcase.blend'))


main()
