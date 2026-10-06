# Video Hub

本地视频浏览与播放：Go 单进程提供 API 与前端，支持 Docker。请上传 **HTML5 可播放** 格式（如 H.264 MP4 / WebM）；服务端不做 AVI/RMVB 转码。

## 快速开始

```bash
mkdir -p videos
export VIDEOHUB_VIDEO_DIR=./videos
go run -buildvcs=false ./cmd/videohub
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

默认将 `./videos` 挂到 `/videos`。容器启动时会按 `PUID`/`PGID`（默认 `1000:1000`）校正挂载目录属主。

## API

- `GET /api/health` — 健康检查
- `GET /api/browse?path=` — 当前目录（文件夹 + 视频）
- `GET /api/search?q=` — 全局搜索（最多 200 条）
- `GET /api/videos` — 全量列表（兼容）
- `POST /api/videos` — 上传（`file`，可选 `path`）
- `GET /api/stream/{id...}` — 原文件流（Range）；AVI/RMVB 返回 `503`

> 无鉴权，请仅在受信任的内网使用，勿直接暴露公网。

设计说明见 [docs/DESIGN.md](docs/DESIGN.md)。
