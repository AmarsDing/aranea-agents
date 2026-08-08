# -*- coding: utf-8 -*-
"""rerender_rack.py — 仅重渲机柜验收图(拉远构图)"""
import bpy, os
from mathutils import Vector

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'models')
scn = bpy.context.scene
cam = bpy.data.objects['Camera']
scn.camera = cam
try:
    scn.render.engine = 'BLENDER_EEVEE_NEXT'
except Exception:
    pass
scn.render.resolution_x = 1280
scn.render.resolution_y = 720

target = Vector((0.0, 0.0, 0.95))

def shot(loc, path, lens=55):
    cam.location = loc
    cam.data.lens = lens
    cam.rotation_euler = (target - Vector(loc)).to_track_quat('-Z', 'Y').to_euler()
    scn.render.filepath = path
    bpy.ops.render.render(write_still=True)
    print('[rack] rendered', os.path.basename(path))

shot((3.4, -4.4, 2.4), os.path.join(OUT, 'preview_rack_front.png'))
shot((-3.4, 4.4, 2.4), os.path.join(OUT, 'preview_rack_rear.png'))
