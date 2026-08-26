# aranea-sandbox-base (M82 P1-3): session-sandbox base image.
# Baseline = python:3.11-slim (same as the P0 profile runtime) + Node 20 LTS
# + git/curl + common Python libs, running as non-root USER sandbox (uid 1000).
#
# Engine contract (internal/sandbox/engine_docker.go):
#   --read-only + --tmpfs /tmp + anonymous volume at /workspace/out.
# A fresh anonymous volume inherits ownership from the image directory, so
# /workspace/out MUST be pre-created and chowned to sandbox here — otherwise
# artifact writes and sandbox_fs_write fail for the non-root user.
# NOTE: pin to the plain `3.11-slim` tag (currently Debian trixie) — the local
# daemon's registry mirror 403s on -bookworm variant pulls, and `3.11-slim` is
# the tag already cached on dev/CI hosts.
FROM python:3.11-slim AS runtime
FROM node:20-slim AS node

FROM runtime
# Node 20 LTS: copy the official slim tree (reproducible, no apt repo dependency).
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
 && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1

# Mirror overrides (build-time only): CN dev hosts hit 502/403 on
# deb.debian.org / pypi.org, so build-sandbox-base.ps1 passes Aliyun mirrors
# by default. Empty = upstream defaults (portable).
ARG APT_MIRROR=""
ARG PIP_INDEX_URL=""

# git/curl: common code-exec tooling; libstdc++6: node runtime dependency.
RUN if [ -n "$APT_MIRROR" ]; then \
      sed -i "s|http://deb.debian.org|$APT_MIRROR|g" /etc/apt/sources.list.d/debian.sources; \
    fi \
 && apt-get update \
 && apt-get install -y --no-install-recommends bash git curl ca-certificates libstdc++6 \
 && rm -rf /var/lib/apt/lists/*

# Common Python libs. The default profile is network=none, so runtime pip
# installs are impossible; the common set is baked in. Keep it lean — the
# image backs every warm-pool instance.
RUN pip install --no-cache-dir ${PIP_INDEX_URL:+-i "$PIP_INDEX_URL"} numpy pandas requests

# Non-root user + artifact-volume ownership (see header note).
RUN groupadd -r -g 1000 sandbox \
 && useradd -r -u 1000 -g sandbox -m -s /bin/bash sandbox \
 && mkdir -p /workspace/out \
 && chown -R sandbox:sandbox /workspace

USER sandbox
WORKDIR /tmp
