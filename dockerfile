# 阶段1：构建前端
FROM node:22-alpine AS web-builder
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 阶段2：构建 Go 后端
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 将前端产物复制到 web/dist，供 go:embed 嵌入
COPY --from=web-builder /app/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o wechatread-client ./cmd/client

# 阶段3：最终镜像
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=go-builder /app/wechatread-client ./
# 数据目录：SQLite 默认落在这里
RUN mkdir -p /data
VOLUME /data
ENV DB_PATH=/data/app.db
ENV LISTEN_ADDR=:8081
EXPOSE 8081
ENTRYPOINT ["./wechatread-client"]
