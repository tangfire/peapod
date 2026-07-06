FROM node:22-alpine AS frontend

WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci --no-audit --no-fund
COPY frontend ./
RUN npm run build

FROM golang:1.25-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/zephyr .

FROM alpine:3.20

RUN apk add --no-cache su-exec && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=builder /out/zephyr /app/zephyr
COPY --from=frontend /src/frontend/dist /app/frontend/dist
RUN mkdir -p /data && chown -R app:app /data
EXPOSE 8095

ENTRYPOINT ["/bin/sh", "-lc", "chown -R app:app /data && exec su-exec app /app/zephyr"]
