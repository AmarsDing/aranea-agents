import bpy

try:
    bpy.ops.preferences.addon_enable(module="blender_mcp")
    bpy.ops.wm.save_userpref()
    print("[boot] blender_mcp addon enabled, server auto-starting on port 9876")
except Exception as e:
    print("[boot] enable failed:", e)
