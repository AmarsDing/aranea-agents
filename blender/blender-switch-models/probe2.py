# -*- coding: utf-8 -*-
"""probe2.py — 灯光贡献分离: 全灭 -> 逐灯累加, 各渲一张小图"""
import sys, os
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy
from mathutils import Vector

ROOT = r'f:\aranea-agents\test\blender-switch-models'
LOG = os.path.join(ROOT, 'models', 'probe_log.txt')


def log(m):
    print('[probe2]', m)
    with open(LOG, 'a', encoding='utf-8') as f:
        f.write('\n[probe2] ' + str(m))


def look_at(o, target):
    d = (Vector(target) - o.location).normalized()
    o.rotation_euler = d.to_track_quat('-Z', 'Y').to_euler()


sc = bpy.context.scene
sc.compositing_node_group = None  # 无辉光
try:
    sc.render.engine = 'BLENDER_EEVEE'
except Exception:
    pass
sc.render.resolution_x, sc.render.resolution_y = (960, 540)
sc.render.resolution_percentage = 100

cam = bpy.data.objects['SWCam']
cam.location = (-0.84, -0.68, 0.30)
cam.data.lens = 40
look_at(cam, (-0.53, -0.15, -0.015))
sc.camera = cam

LIGHTS = ['SWKey', 'SWFill', 'SWRim', 'SWTop', 'SWFront', 'SWAccent']
saved = {}
for n in LIGHTS:
    o = bpy.data.objects.get(n)
    if o:
        saved[n] = o.data.energy
        o.data.energy = 0.0
log('world strength: %s' % sc.world.node_tree.nodes['Background'].inputs[1].default_value)
log('world color: %s' % (tuple(sc.world.node_tree.nodes['Background'].inputs[0].default_value),))

bpy.context.view_layer.update()
sc.render.filepath = os.path.join(ROOT, 'probe_l0_none.png')
bpy.ops.render.render(write_still=True)
log('rendered L0 (all off)')

for n in ('SWTop', 'SWKey'):
    o = bpy.data.objects.get(n)
    if o:
        o.data.energy = saved[n]
    bpy.context.view_layer.update()
    sc.render.filepath = os.path.join(ROOT, 'probe_l1_%s.png' % n.lower())
    bpy.ops.render.render(write_still=True)
    log('rendered +%s (%.1fW)' % (n, saved.get(n, -1)))
log('DONE')
