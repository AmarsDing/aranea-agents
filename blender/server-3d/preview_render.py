# Preview-only re-render from saved blend (wide framing). Run: blender -b aranea_servers.blend --python preview_render.py
import bpy, os, math
from mathutils import Vector

OUT_DIR = r'f:\aranea-agents\test\server-3d'

# clean non-server objects
for o in list(bpy.data.objects):
    if not (o.name.startswith('SRV_') or o.name.startswith('Tmp')):
        bpy.data.objects.remove(o, do_unlink=True)

cd = bpy.data.cameras.new('TmpCam')
cam = bpy.data.objects.new('TmpCam', cd)
bpy.context.scene.collection.objects.link(cam)
sd = bpy.data.lights.new('TmpSun', type='SUN')
sd.energy = 2.5
sd.angle = math.radians(30)
sun = bpy.data.objects.new('TmpSun', sd)
bpy.context.scene.collection.objects.link(sun)
sun.rotation_euler = (math.radians(50), math.radians(15), math.radians(30))

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

bpy.context.scene.frame_set(100)
cam.location = (0.05, -5.2, 2.2)
cam.rotation_euler = (Vector((0.05, 0.0, 0.22)) - cam.location).to_track_quat('-Z', 'Y').to_euler()
safe_render(os.path.join(OUT_DIR, 'preview_exploded.png'))
print('PREVIEW DONE')
