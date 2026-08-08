import sys
import bpy

sys.path.insert(0, r"F:\aranea-agents\test\blender-rack")

import addon

addon.register()

bpy.context.scene.blendermcp_port = 9876
bpy.ops.blendermcp.start_server()

print("[bootstrap] BlenderMCP server start requested on port 9876")
