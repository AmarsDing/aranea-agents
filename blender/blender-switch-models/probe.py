# -*- coding: utf-8 -*-
"""probe.py — 诊断: Glare 节点 API + showcase 相机射线 + workbench 取景验证"""
import sys, os
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy
from mathutils import Vector

ROOT = r'f:\aranea-agents\test\blender-switch-models'
LOG = os.path.join(ROOT, 'models', 'probe_log.txt')
lines = []


def log(m):
    print('[probe]', m)
    lines.append(str(m))
    with open(LOG, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))


def look_at(o, target):
    d = (Vector(target) - o.location).normalized()
    o.rotation_euler = d.to_track_quat('-Z', 'Y').to_euler()


sc = bpy.context.scene
deps = bpy.context.evaluated_depsgraph_get()

# 1. Glare 节点可用属性
try:
    ng = bpy.data.node_groups.new('ProbeComp', 'CompositorNodeTree')
    gl = ng.nodes.new('CompositorNodeGlare')
    attrs = [p.identifier for p in gl.bl_rna.properties if not p.is_readonly]
    log('Glare attrs: %s' % attrs)
    log('Glare inputs: %s' % [i.name for i in gl.inputs])
    # 5.x 新 API?
    if hasattr(gl, 'node_tree'):
        log('Glare has node_tree (group-based)')
    bpy.data.node_groups.remove(ng)
except Exception as e:
    log('Glare probe failed: %s' % e)

# 2. showcase 相机位姿 + 射线检测
cam = bpy.data.objects['SWCam']
cam.location = (0.02, -1.90, 0.95)
cam.data.lens = 35
look_at(cam, (0.02, -0.02, -0.03))
bpy.context.view_layer.update()
log('cam loc %s rot %s' % (tuple(round(v, 3) for v in cam.location),
                           tuple(round(v, 3) for v in cam.rotation_euler)))
fwd = cam.matrix_world.to_quaternion() @ Vector((0, 0, -1))
log('cam forward: %s' % (tuple(round(v, 3) for v in fwd),))
for name, off in (('center', (0, 0, 0)), ('left', (-0.3, 0, 0)), ('right', (0.3, 0, 0))):
    origin = cam.location + Vector(off)
    hit, loc, norm, idx, obj, mat = sc.ray_cast(deps, origin, fwd.normalized())
    log('ray %s: hit=%s obj=%s loc=%s' % (
        name, hit, obj.name if obj else None,
        tuple(round(v, 3) for v in loc) if hit else None))

# 3. workbench 平渲验证取景
sc.render.engine = 'BLENDER_WORKBENCH'
sc.render.resolution_x, sc.render.resolution_y = (1280, 720)
sc.render.resolution_percentage = 100
sc.render.filepath = os.path.join(ROOT, 'probe_wb_showcase.png')
bpy.ops.render.render(write_still=True)
log('workbench render done')
log('DONE')
