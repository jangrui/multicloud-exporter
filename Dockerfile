FROM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOTOOLCHAIN=local

# 构建参数
ARG VERSION=dev
ARG COMMIT_SHA=none
ARG BUILD_TIME=unknown

COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-w -s -X 'multicloud-exporter/internal/version.Version=${VERSION}' -X 'multicloud-exporter/internal/version.CommitSHA=${COMMIT_SHA}' -X 'multicloud-exporter/internal/version.BuildTime=${BUILD_TIME}'" \
    -o multicloud-exporter ./cmd/multicloud-exporter

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/multicloud-exporter .
COPY --from=builder /app/configs/mappings ./configs/mappings
COPY --from=builder /app/configs/server.yaml ./configs/server.yaml

ENV SERVER_PATH=/app/configs/server.yaml
ENV PRODUCTS_PATH=/app/configs/products.yaml
ENV EXPORTER_PORT=${EXPORTER_PORT:-9101}

EXPOSE ${EXPORTER_PORT:-9101}

# 创建数据目录并设置正确的所有者权限
# 非特权用户（UID 65532）需要有写入权限以保存区域状态
RUN mkdir -p /app/data && \
    chown -R 65532:65532 /app/data

USER 65532:65532

ENTRYPOINT ["./multicloud-exporter"]
