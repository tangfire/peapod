# syntax=docker/dockerfile:1.7

FROM node:22-alpine AS frontend

WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY frontend ./
RUN npm run build

FROM golang:1.25-alpine AS builder

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/peapod .

FROM alpine:3.20

RUN apk add --no-cache su-exec docker-cli && adduser -D -H -u 10001 app
WORKDIR /app
COPY --from=builder /out/peapod /app/peapod
COPY --from=frontend /src/frontend/dist /app/frontend/dist
RUN mkdir -p /data && chown -R app:app /data
EXPOSE 8095

ENTRYPOINT ["/bin/sh", "-lc", "chown -R app:app /data && exec su-exec app /app/peapod"]
