FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /out/arenea-server ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/arenea-server /app/arenea-server
EXPOSE 8080
CMD ["/app/arenea-server"]
