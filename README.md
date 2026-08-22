# Video Hub

简单的本地 MP4 视频浏览与播放服务：Go 单进程提供 API 与前端页面，支持 Docker 部署。

## 快速开始

```bash
# 准备视频目录（默认 ./videos）
mkdir -p videos
# 放入一些 .mp4 文件后启动
export VIDEOHUB_VIDEO_DIR=./videos
go run ./cmd/videohub
```

浏览器打开 http://localhost:8080

## 配置（环境变量）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `VIDEOHUB_ADDR` | `:8080` | HTTP 监听地址 |
| `VIDEOHUB_VIDEO_DIR` | `./videos` | 视频根目录 |
| `VIDEOHUB_SCAN_DEPTH` | `0` | 扫描深度，`0` 表示不限制 |
| `VIDEOHUB_LIST_CACHE_SEC` | `5` | 列表缓存秒数，`0` 关闭缓存 |
| `VIDEOHUB_MAX_UPLOAD_MB` | `2048` | 单文件上传上限（MiB） |

## Docker

```bash
mkdir -p videos
docker compose up --build
```

默认将 `./videos` 以读写方式挂载到容器内 `/videos`（上传需要写权限）。

容器启动时会按 `PUID`/`PGID`（默认 `1000:1000`）校正挂载目录属主，避免上传出现 `permission denied`。若你的宿主机用户不是 1000，请在 `docker-compose.yml` 里改成自己的 uid/gid（`id -u` / `id -g`）。

## API

- `GET /api/health` — 健康检查
- `GET /api/videos` — 视频列表
- `POST /api/videos` — 上传 MP4（`multipart` 字段 `file`）
- `GET /api/stream/{id...}` — MP4 流（支持 Range）

> 无鉴权，请仅在受信任的内网使用，勿直接暴露公网。

设计说明见 [docs/DESIGN.md](docs/DESIGN.md)。
