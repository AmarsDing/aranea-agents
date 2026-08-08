# One-click finalize: rebuild all servers -> export FBX (UE5) -> write ue5_integration.json
# -> save .blend copy -> render previews. Run inside Blender (Scripting workspace > Open > Run Script).
import bpy, os, json, math
from mathutils import Vector

OUT_DIR = r'f:\aranea-agents\test\server-3d'
os.makedirs(OUT_DIR, exist_ok=True)

# clean scene (factory startup defaults etc.)
for o in list(bpy.data.objects):
    bpy.data.objects.remove(o, do_unlink=True)

exec(open(r'f:\aranea-agents\test\server-3d\build_all.py').read())
r1, r2, r4 = build_all()
print('rebuilt:', len(r1.children_recursive), len(r2.children_recursive), len(r4.children_recursive))

# ---------- FBX export (one file per server, selection = root hierarchy) ----------
# SRV_TAGS env: comma subset e.g. "2U,4U" to skip slow bakes when only some models changed
# SRV_JSON_ONLY=1: skip FBX export + renders, only regenerate ue5_integration.json
JSON_ONLY = os.environ.get('SRV_JSON_ONLY') == '1'
EXPORT_TAGS = [] if JSON_ONLY else os.environ.get('SRV_TAGS', '1U,2U,4U').split(',')
for tag in EXPORT_TAGS:
    root = bpy.data.objects['SRV_%s_ROOT' % tag]
    bpy.ops.object.select_all(action='DESELECT')
    stack = [root]
    while stack:
        o = stack.pop()
        o.select_set(True)
        stack.extend(o.children)
    bpy.context.view_layer.objects.active = root
    fp = os.path.join(OUT_DIR, 'SRV_%s.fbx' % tag)
    bpy.ops.export_scene.fbx(filepath=fp, use_selection=True,
                             use_custom_props=True, bake_anim=True)
    print('exported', fp)

# ---------- UE5 integration metadata ----------
def dump_server(root):
    items = []
    for o in [root] + list(root.children_recursive):
        props = {k: (list(v) if not isinstance(v, (str, int, float, bool)) else v)
                 for k, v in o.items()}
        items.append({'name': o.name, 'type': o.type,
                      'parent': o.parent.name if o.parent else None,
                      'material': (o.data.materials[0].name if o.type == 'MESH' and o.data.materials else None),
                      'props': props})
    return items

def server_summary(root):
    """装配契约: 整机尺寸(根节点局部空间) + 端口/UI 锚点。
    local 约定: 前面 y=-D/2, 后面 y=+D/2, 底 z=0, 顶 z=U*44.45mm-1mm。"""
    bpy.context.view_layer.update()   # 确保 matrix_world 非陈旧
    inv = root.matrix_world.inverted()
    lo = Vector((1e9, 1e9, 1e9))
    hi = Vector((-1e9, -1e9, -1e9))
    ports = []
    for o in root.children_recursive:
        if o.type == 'MESH':
            for c in o.bound_box:
                p = inv @ (o.matrix_world @ Vector(c))
                lo = Vector(map(min, lo, p))
                hi = Vector(map(max, hi, p))
        cat = o.get('hw_category')
        if cat in ('PORT', 'PSU') or o.get('led_type') == 'nic':
            p = inv @ o.matrix_world.translation
            ports.append({'name': o.name, 'category': cat or 'LED_NIC',
                          'desc': o.get('hw_name') or o.get('led_desc') or '',
                          'bind': o.get('data_bind', ''),
                          'pos_local': [round(v, 5) for v in p]})
    dims = [round(hi[i] - lo[i], 5) for i in range(3)]
    return {'u_count': root.get('server_u'),
            'dims_m': {'width': dims[0], 'depth': dims[1], 'height': dims[2]},
            'mount': {'front_y_local': round(lo[1], 5), 'bottom_z_local': round(lo[2], 5),
                      'formula': 'rack 装配: root.z = u_bottom_z(u); root.y = rack_rail_front_y + D/2'},
            'ports': sorted(ports, key=lambda p: p['name'])}

meta = {
    'units': 'meters in Blender; FBX exported with unit scale -> UE5 centimeters (1:1 real size)',
    'coordinate': 'front=-Y, up=+Z in Blender; FBX default export axes',
    'animation': {
        'frame_1': 'assembled', 'frame_100': 'exploded',
        'note': 'UE5 recommended: drive explode procedurally via explode_dx/dy/dz custom props '
                '(lerp home -> home+offset with a Timeline). FBX baked object animation is also included.'},
    'led_materials': {
        'M_LED_Power':  {'bind': 'server.power',  'states': {'on': 'green', 'off': 'off', 'standby': 'amber'}},
        'M_LED_Health': {'bind': 'server.health', 'states': {'ok': 'green', 'warning': 'amber', 'critical': 'red'}},
        'M_LED_Alert':  {'bind': 'server.alert',  'states': {'none': 'off', 'fault': 'amber blink'}},
        'M_LED_UID':    {'bind': 'server.uid',    'states': {'on': 'blue', 'off': 'off'}},
        'M_LED_NIC':    {'bind': 'server.nics[i].activity', 'states': {'link': 'green', 'activity': 'green blink', 'down': 'off'}},
        'M_LED_Disk':   {'bind': 'server.disks[i].activity', 'states': {'activity': 'green blink', 'fault': 'amber', 'off': 'off'}},
    },
    'led_usage': 'UE5: per LED object create a Material Instance Dynamic from its M_LED_* material, '
                 'set Emissive Color/Intensity from monitoring data. Each LED object custom props carry '
                 'led_type / led_desc / data_bind.',
    'hw_props': 'Every hardware object carries custom props hw_category/hw_name/hw_model/hw_spec. '
                'FBX use_custom_props exports them; UE5 imports as user-defined metadata for UI display.',
    'servers': {},
    'assembly': {},
}
for tag, root in (('1U', r1), ('2U', r2), ('4U', r4)):
    meta['servers'][tag] = dump_server(root)
    meta['assembly'][tag] = server_summary(root)
with open(os.path.join(OUT_DIR, 'ue5_integration.json'), 'w', encoding='utf-8') as f:
    json.dump(meta, f, ensure_ascii=False, indent=1)
print('json written')

# ---------- blend copy ----------
if not JSON_ONLY:
    bpy.ops.wm.save_as_mainfile(filepath=os.path.join(OUT_DIR, 'aranea_servers.blend'), copy=True)
    print('blend copy saved')

# ---------- preview renders ----------
if JSON_ONLY:
    print('FINALIZE DONE (json only)')
    raise SystemExit
if 'TmpCam' not in bpy.data.objects:
    cd = bpy.data.cameras.new('TmpCam')
    cam = bpy.data.objects.new('TmpCam', cd)
    bpy.context.scene.collection.objects.link(cam)
else:
    cam = bpy.data.objects['TmpCam']
if 'TmpSun' not in bpy.data.objects:
    sd = bpy.data.lights.new('TmpSun', type='SUN')
    sd.energy = 2.5
    sun = bpy.data.objects.new('TmpSun', sd)
    bpy.context.scene.collection.objects.link(sun)
    sun.rotation_euler = (math.radians(50), math.radians(15), math.radians(30))
for o in bpy.data.objects:
    if not (o.name.startswith('SRV_') or o.name.startswith('Tmp') or o.type in ('LIGHT', 'CAMERA') or o.name == 'Plane'):
        o.hide_render = True
scn = bpy.context.scene
scn.camera = cam
scn.render.resolution_x = 1280
scn.render.resolution_y = 720
try:
    scn.render.engine = 'BLENDER_EEVEE_NEXT'
except Exception:
    try:
        scn.render.engine = 'BLENDER_EEVEE'
    except Exception:
        pass

# assembled view
def safe_render(path):
    try:
        scn.render.filepath = path
        bpy.ops.render.render(write_still=True)
        print('rendered', path)
    except Exception as e:
        print('render skipped:', e)

bpy.context.scene.frame_set(1)
cam.location = (0.05, -4.6, 1.6)
cam.rotation_euler = (Vector((0.05, 0.0, 0.05)) - cam.location).to_track_quat('-Z', 'Y').to_euler()
safe_render(os.path.join(OUT_DIR, 'preview_servers.png'))

# exploded view (frame 100)
bpy.context.scene.frame_set(100)
cam.location = (0.05, -5.2, 2.2)
cam.rotation_euler = (Vector((0.05, 0.0, 0.22)) - cam.location).to_track_quat('-Z', 'Y').to_euler()
safe_render(os.path.join(OUT_DIR, 'preview_exploded.png'))
bpy.context.scene.frame_set(1)

print('FINALIZE DONE')
