# 基础镜像可用 --build-arg 覆盖：内网/受限网络环境用本地已缓存的等价 tag 构建
# （如 NODE_IMAGE=node:20-bookworm NGINX_IMAGE=nginx:1.28.1-alpine），默认值不变
ARG NODE_IMAGE=node:22-alpine
ARG NGINX_IMAGE=nginx:1.27-alpine
FROM ${NODE_IMAGE} AS builder
WORKDIR /app
COPY package.json ./
# 国内构建环境走 npmmirror（2026-08-29 实证：直连 npmjs 容器内 12min+ 装不完）；
# npm install 非 ci 是历史口径（package.json 与 package-lock 偶尔漂移），不动
RUN npm config set registry https://registry.npmmirror.com && npm install
COPY . .
RUN npm run build

ARG NGINX_IMAGE
FROM ${NGINX_IMAGE}
# Quasar 产物在 dist/spa/（quasar.config 未设 distDir）；直接拷 /app/dist 会残留 nginx 默认欢迎页
COPY --from=builder /app/dist/spa /usr/share/nginx/html
# SPA history 回退 + /v1 /v2 /api /healthz 同源反代到 compose 内 aranea-admin:8810
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
