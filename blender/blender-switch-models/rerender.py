# -*- coding: utf-8 -*-
"""rerender.py — 在已保存的 switch_showcase.blend 上重渲染（修复相机/取景/辉光）
用法: blender models\\switch_showcase.blend --python rerender.py
"""
import sys, os, math, traceback
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy
from mathutils import Vector

ROOT = r'f:\aranea-agents\test\blender-switch-models'
LOG = os.path.join(ROOT, 'models', 'export_log.txt')


def log(msg):
    print('[rerender]', msg)
    with open(LOG, 'a', encoding='utf-8') as f:
        f.write('\n[rerender] ' + str(msg))


def look_at(o, target):
    d = (Vector(target) - o.location).normalized()
    o.rotation_euler = d.to_track_quat('-Z', 'Y').to_euler()


def strip_constraints(names):
    for n in names:
        o = bpy.data.objects.get(n)
        if o:
            for c in list(o.constraints):
                o.constraints.remove(c)


def setup_lights():
    specs = {  # name: (location, target, energy_w)
        'SWKey':  ((-0.95, -1.25, 1.45), (0.0, -0.1, -0.02), 8),
        'SWFill': ((1.15, -0.75, 0.85), (0.1, -0.15, -0.02), 5),
        'SWRim':  ((0.20, 0.95, 1.05), (0.0, -0.1, 0.0), 10),
        'SWTop':  ((0.00, 0.10, 1.85), (0.0, 0.0, -0.02), 3),
    }
    for n, (loc, tgt, energy) in specs.items():
        o = bpy.data.objects.get(n)
        if o:
            o.location = loc
            o.data.energy = energy
            look_at(o, tgt)
    # 正面补光: 照亮面板/端口细节
    if not bpy.data.objects.get('SWFront'):
        ld = bpy.data.lights.new('SWFront', 'AREA')
        ld.color = (0.88, 0.94, 1.00)
        ld.shape = 'DISK'
        ld.size = 0.9
        o = bpy.data.objects.new('SWFront', ld)
        bpy.context.scene.collection.objects.link(o)
    fo = bpy.data.objects['SWFront']
    fo.data.energy = 8
    fo.location = (0.02, -1.45, 0.38)
    look_at(fo, (0.02, -0.21, -0.01))
    # 低角度蓝色氛围光 (抬高防地面热点)
    ac = bpy.data.objects.get('SWAccent')
    if ac:
        ac.data.energy = 3
        ac.location = (0.02, -0.42, 0.10)
    # 机壳: 哑光喷粉钢 (低金属度, 防顶灯镜面反射发白)
    for mn, metal, rough in (('M_Chassis', 0.30, 0.55), ('M_Panel', 0.40, 0.50), ('M_Fin', 0.75, 0.50)):
        m = bpy.data.materials.get(mn)
        if m:
            b = m.node_tree.nodes['Principled BSDF']
            b.inputs['Metallic'].default_value = metal
            b.inputs['Roughness'].default_value = rough
    # 地面降低反光防过曝
    g = bpy.data.objects.get('SWGround')
    if g and g.data.materials:
        b = g.data.materials[0].node_tree.nodes['Principled BSDF']
        b.inputs['Metallic'].default_value = 0.20
        b.inputs['Roughness'].default_value = 0.60


def add_bloom(sc):
    """Blender 5.x: scene.compositing_node_group; Glare 属性已改为输入插口"""
    try:
        ng = bpy.data.node_groups.get('SWComp')
        if ng:
            bpy.data.node_groups.remove(ng)
        ng = bpy.data.node_groups.new('SWComp', 'CompositorNodeTree')
        ng.interface.new_socket(name='Image', in_out='INPUT', socket_type='NodeSocketColor')
        ng.interface.new_socket(name='Image', in_out='OUTPUT', socket_type='NodeSocketColor')
        ng.nodes.new('NodeGroupInput')
        outp = ng.nodes.new('NodeGroupOutput')
        rl = ng.nodes.new('CompositorNodeRLayers')
        gl = ng.nodes.new('CompositorNodeGlare')
        gl.inputs['Type'].default_value = 'Fog Glow'
        gl.inputs['Quality'].default_value = 'High'
        gl.inputs['Threshold'].default_value = 0.8
        gl.inputs['Size'].default_value = 7
        ng.links.new(rl.outputs['Image'], gl.inputs['Image'])
        ng.links.new(gl.outputs['Image'], outp.inputs['Image'])
        sc.compositing_node_group = ng
        log('bloom: compositing_node_group OK')
        return True
    except Exception as e:
        log('bloom failed: %s' % e)
        return False


def shot(cam, cam_loc, tgt_loc, path, res, lens):
    cam.location = cam_loc
    cam.data.lens = lens
    look_at(cam, tgt_loc)
    sc = bpy.context.scene
    sc.render.resolution_x, sc.render.resolution_y = res
    sc.render.resolution_percentage = 100
    sc.render.image_settings.file_format = 'PNG'
    sc.render.filepath = path
    bpy.context.view_layer.update()
    bpy.ops.render.render(write_still=True)
    log('rendered %s' % os.path.basename(path))


def main():
    sc = bpy.context.scene
    # 清除启动默认物体 (2m Cube 曾包裹整个场景导致全景全白)
    for n in ('Cube', 'Light', 'Camera'):
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
            log('removed default object: %s' % n)
    strip_constraints(['SWKey', 'SWFill', 'SWRim', 'SWTop', 'SWFront', 'SWCam'])
    setup_lights()
    try:
        sc.render.engine = 'BLENDER_EEVEE'
    except Exception:
        sc.render.engine = 'BLENDER_EEVEE_NEXT'
    try:
        sc.eevee.taa_render_samples = 64
        sc.eevee.use_raytracing = True
    except Exception:
        pass
    cam = bpy.data.objects['SWCam']
    sc.camera = cam
    bpy.context.view_layer.update()
    # 先出一张无辉光对照图
    sc.compositing_node_group = None
    shot(cam, (0.02, -1.90, 0.95), (0.02, -0.02, -0.03),
         os.path.join(ROOT, 'showcase_raw.png'), (1920, 1080), 35)
    add_bloom(sc)
    # 全景: 35mm 广角覆盖三台 (总宽 ~1.6m)
    shot(cam, (0.02, -1.90, 0.95), (0.02, -0.02, -0.03),
         os.path.join(ROOT, 'showcase.png'), (1920, 1080), 35)
    # 单机 3/4 前视
    shot(cam, (-0.84, -0.68, 0.30), (-0.53, -0.15, -0.015),
         os.path.join(ROOT, 'preview_sw1.png'), (1600, 900), 40)
    shot(cam, (-0.30, -0.72, 0.30), (0.0, -0.15, -0.015),
         os.path.join(ROOT, 'preview_sw2.png'), (1600, 900), 40)
    shot(cam, (0.88, -0.78, 0.32), (0.56, -0.15, -0.015),
         os.path.join(ROOT, 'preview_sw3.png'), (1600, 900), 40)
    # RJ45 微距 (SW2 中部端口 + LED)
    shot(cam, (-0.075, -0.42, 0.048), (-0.055, -0.212, 0.002),
         os.path.join(ROOT, 'preview_jack.png'), (1600, 900), 70)
    bpy.ops.wm.save_as_mainfile(filepath=bpy.data.filepath)
    log('blend saved, DONE')


try:
    main()
except Exception:
    log('FATAL:\n' + traceback.format_exc())
    raise
