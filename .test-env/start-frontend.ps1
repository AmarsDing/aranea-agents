# 独立测试环境前端启动脚本
# 端口: 19001, 代理到测试后端 18000
$env:QUASAR_DEV_PORT = "19001"
$env:QUASAR_BACKEND_URL = "http://127.0.0.1:18000"
Set-Location "F:\aranea-agents\web"
pnpm dev
