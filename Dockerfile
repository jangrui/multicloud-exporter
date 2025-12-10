FROM golang:1.25-alpine AS builder
WORKDIR /app
ENV GOTOOLCHAIN=auto
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o multicloud-exporter ./cmd/multicloud-exporter

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/multicloud-exporter .
COPY --from=builder /app/configs/mappings/lb.metrics.yaml ./configs/mappings/lb.metrics.yaml
COPY --from=builder /app/configs/server.yaml ./configs/server.yaml

ENV SERVER_PATH=/app/configs/server.yaml
ENV PRODUCTS_PATH=/app/configs/products.yaml
ENV EXPORTER_PORT=${EXPORTER_PORT}

EXPOSE ${EXPORTER_PORT}

USER 65532:65532
ENTRYPOINT ["./multicloud-exporter"]
