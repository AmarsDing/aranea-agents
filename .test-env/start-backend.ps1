# 独立测试环境后端启动脚本
# 端口: HTTP 18000 / gRPC 19000 / WS 18002, 数据库: aranea_test
$env:KRATOS_HTTP_AUTH_DISABLED = "1"
$env:DEPLOY_ENV = "dev"
$env:DAO_VECTOR_PGVECTOR = "1"
& "F:\aranea-agents\bin\admin-test.exe" -conf "F:\aranea-agents\.test-env\config\config.yaml"
