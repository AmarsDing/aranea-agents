#!/bin/bash
set -euo pipefail
# Aranea 离线包一键安装（Linux 目标机）：docker load → web-config/.env 注入 → compose up → 冒烟验证。
# 用法：把导出目录整体拷贝到目标机任意位置，在本目录执行 bash install.sh --server-ip <本机IP>
# 前置：Docker Engine + compose plugin 已就绪；8810/9910/8812/9301 端口空闲。
PKG_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE="$PKG_DIR/docker-compose.yaml"
IMAGES_TAR="$PKG_DIR/images.tar"
SERVER_IP=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-ip) SERVER_IP="$2"; shift 2 ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done

step() { echo -e "\n== $1 =="; }
ok()   { echo -e "  \e[32m$1\e[0m"; }
die()  { echo -e "  \e[31m$1\e[0m" >&2; exit 1; }

step '0. 前置自检（docker / 端口 / 包完整性）'
docker info >/dev/null 2>&1 || die 'Docker daemon 未就绪'
[[ -f "$IMAGES_TAR" ]] || die "images.tar 缺失: $IMAGES_TAR"
[[ -f "$COMPOSE" ]]    || die "docker-compose.yaml 缺失: $COMPOSE"
[[ -n "$SERVER_IP" ]] || die '缺少 --server-ip <本机IP>（用于前端 runtime-config 与 CORS）'
for p in 8810 9910 8812 9301; do
  if (echo >"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
    die "端口被占用: $p（请先释放）"
  fi
done

step '1. docker load 镜像（体积大，耗时较长）'
docker load -i "$IMAGES_TAR" >/dev/null || die 'docker load 失败'

step '2. 注入目标机地址（web-config/runtime-config.json + .env）'
cat > "$PKG_DIR/web-config/runtime-config.json" <<EOF
{
  "backendUrl": "http://${SERVER_IP}:8810",
  "wsOrigin": "http://${SERVER_IP}:8810"
}
EOF
echo "ARANEA_SERVER_IP=${SERVER_IP}" > "$PKG_DIR/.env"
ok "runtime-config → http://${SERVER_IP}:8810"

step '3. compose up -d'
docker compose -f "$COMPOSE" --project-directory "$PKG_DIR" up -d >/dev/null || die 'compose up 失败'

step '4. 冒烟验证（healthz / web）'
deadline=$(( $(date +%s) + 120 ))
while [[ $(date +%s) -lt $deadline ]]; do
  if curl -sf -o /dev/null "http://127.0.0.1:8810/healthz"; then break; fi
  sleep 3
done
curl -sf -o /dev/null "http://127.0.0.1:8810/healthz" || die 'admin healthz 验证失败（120s 超时）'
ok 'admin healthz OK'
curl -sf -o /dev/null "http://127.0.0.1:9301/" || die 'web 9301 验证失败'
ok 'web 9301 OK'

echo -e "\n安装完成：前端 http://${SERVER_IP}:9301  后端 http://${SERVER_IP}:8810"
echo "默认管理员：admin / changeme（首登后请立即修改；DATA__INITIAL_ADMIN__PASSWORD 可在 compose 覆盖）"
