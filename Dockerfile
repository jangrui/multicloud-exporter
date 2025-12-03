FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o multicloud-exporter ./cmd/multicloud-exporter

FROM alpine:latest
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/multicloud-exporter .

ENV EXPORTER_PORT=9101
ENV SCRAPE_INTERVAL=60
ENV SERVER_PATH=/app/configs/server.yaml
ENV PRODUCTS_PATH=/app/configs/products.yaml

EXPOSE 9101
USER 65532:65532
ENTRYPOINT ["./multicloud-exporter"]
