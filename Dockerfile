FROM golang:1.23 AS builder

COPY . /src
WORKDIR /src

# GO_BUILD_TAGS is empty for the default (main-program) image; the evaluation
# compose file passes "pgvector" to enable vector recall in the memoryeval binary.
ARG GO_BUILD_TAGS=""
RUN GOPROXY=https://goproxy.cn GO_BUILD_TAGS="${GO_BUILD_TAGS}" make build

FROM debian:stable-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
		ca-certificates  \
        netbase \
        && rm -rf /var/lib/apt/lists/ \
        && apt-get autoremove -y && apt-get autoclean -y

COPY --from=builder /src/bin /app

WORKDIR /app

EXPOSE 8800
EXPOSE 9900
VOLUME /data/conf

CMD ["./admin", "-conf", "/data/conf"]
