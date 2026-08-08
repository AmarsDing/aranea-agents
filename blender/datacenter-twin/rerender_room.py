# -*- coding: utf-8 -*-
"""rerender_room.py — 室内视角重渲 preview_room.png"""
import bpy, os
from mathutils import Vector

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'models')
scn = bpy.context.scene
try:
    scn.render.engine = 'BLENDER_EEVEE_NEXT'
except Exception:
    pass
scn.render.resolution_x = 1600
scn.render.resolution_y = 900

cam = bpy.data.objects.get('RoomCam')
scn.camera = cam
cam.data.lens = 20
cam.data.clip_start = 0.05
# 室内: 站冷通道看机柜排布区 ( racks 在 y=0 附近 )
cam.location = (-2.2, -2.4, 2.3)
cam.rotation_euler = (Vector((0.35, 0.2, 0.7)) - cam.location).to_track_quat('-Z', 'Y').to_euler()

# 增强室内照明
key = bpy.data.objects.get('RoomKey')
if key:
    key.data.energy = 450
ld2 = bpy.data.lights.new('RoomFill', 'AREA')
ld2.energy = 250
ld2.shape = 'RECTANGLE'
ld2.size = 3.0
ld2.size_y = 3.0
lo2 = bpy.data.objects.new('RoomFill', ld2)
scn.collection.objects.link(lo2)
lo2.location = (-1.5, -1.5, 2.9)

scn.render.filepath = os.path.join(OUT, 'preview_room.png')
bpy.ops.render.render(write_still=True)
print('[room] rendered preview_room.png (interior)')
