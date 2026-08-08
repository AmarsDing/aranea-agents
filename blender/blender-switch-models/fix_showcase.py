# -*- coding: utf-8 -*-
"""fix_showcase.py — 仅重渲染 showcase.png: 修复地面蓝色光斑 + 整体提亮
用法: blender -b models\\switch_showcase.blend --python fix_showcase.py
"""
import sys, os
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy
from mathutils import Vector

ROOT = r'f:\aranea-agents\test\blender-switch-models'


def look_at(o, target):
    d = (Vector(target) - o.location).normalized()
    o.rotation_euler = d.to_track_quat('-Z', 'Y').to_euler()


def main():
    sc = bpy.context.scene
    # 1) 氛围光: 抬高+贴近前面板, 掠射面板而非在地面聚斑
    ac = bpy.data.objects.get('SWAccent')
    if ac:
        ac.data.energy = 1.2
        ac.location = (0.02, -0.62, 0.30)
        look_at(ac, (0.02, -0.21, 0.0))
    # 2) 整体提亮 (宽景偏暗)
    for n, e in (('SWKey', 11), ('SWFill', 7), ('SWTop', 5), ('SWFront', 10)):
        o = bpy.data.objects.get(n)
        if o:
            o.data.energy = e
    # 3) 重渲染 showcase (辉光合成组已保存在 blend 中)
    cam = bpy.data.objects['SWCam']
    sc.camera = cam
    cam.location = (0.02, -1.90, 0.95)
    cam.data.lens = 35
    look_at(cam, (0.02, -0.02, -0.03))
    sc.render.resolution_x, sc.render.resolution_y = 1920, 1080
    sc.render.resolution_percentage = 100
    sc.render.image_settings.file_format = 'PNG'
    sc.render.filepath = os.path.join(ROOT, 'showcase.png')
    bpy.context.view_layer.update()
    bpy.ops.render.render(write_still=True)
    bpy.ops.wm.save_as_mainfile(filepath=bpy.data.filepath)
    print('[fix_showcase] DONE')


main()
