# AGENTS.md

> 面向 AI coding agents 的项目说明（格式参考 [agents.md](https://agents.md/)）。
> 人类读者请先看 [README.md](README.md)；详细设计见 [docs/DESIGN.md](docs/DESIGN.md)。

## Overview

Video Hub 是一个**前后端统一交付**的本地 MP4 视频站：单个 Go 进程同时提供 HTTP API 与静态前端（`embed.FS`），支持 Docker 部署、群晖风格目录浏览与搜索、Range 播放、流式上传（含文件夹批量）。

**非目标（除非用户明确要求）：** 登录鉴权、转码/HLS、数据库、封面截取、公网暴露加固。

## Tech Stack

| 层 | 技术 |
|----|------|
| Backend | Go 1.23+（见 `go.mod`），标准库 `net/http`，无外部业务依赖 |
| Frontend | 原生 HTML / CSS / JS（`web/`），无构建链、无框架 |
| Deliverable | 单二进制；前端经 `web/embed.go` 嵌入 |
| Deploy | Docker 多阶段构建 + `docker-compose.yml`；entrypoint 按 `PUID`/`PGID` 校正挂载目录权限 |

## Project Structure

```
cmd/videohub/          # main：配置、启动 HTTP
internal/config/       # 环境变量配置
internal/library/      # 视频扫描、路径安全、落盘上传
internal/api/          # HTTP handlers（health / list / upload / stream）
web/                   # 前端静态资源 + embed.FS
docs/DESIGN.md         # 设计文档（API、安全、验收）
Dockerfile             # 多阶段镜像
docker-entrypoint.sh   # root 校正权限后 su-exec 降权
docker-compose.yml     # 本地/部署示例
```

## Commands

```bash
# 本地运行（需先有视频目录）
mkdir -p videos
export VIDEOHUB_VIDEO_DIR=./videos
go run -buildvcs=false ./cmd/videohub

# 测试与构建（本仓库磁盘上偶发 VCS 探测卡住时加 -buildvcs=false）
go test -buildvcs=false ./...
go build -buildvcs=false -o videohub ./cmd/videohub

# Docker
docker compose up --build
```

常用环境变量：`VIDEOHUB_ADDR`、`VIDEOHUB_VIDEO_DIR`、`VIDEOHUB_SCAN_DEPTH`、`VIDEOHUB_LIST_CACHE_SEC`、`VIDEOHUB_MAX_UPLOAD_MB`、`PUID`/`PGID`（容器写权限）。

## API（勿随意改路径）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/health` | 健康检查 |
| `GET` | `/api/browse` | 当前目录：`?path=`（空=根），`ReadDir`，无 DB |
| `GET` | `/api/search` | 搜索：`?q=`，基于 `List()` 缓存，默认最多 200 |
| `GET` | `/api/videos` | 全量列表 JSON |
| `POST` | `/api/videos` | 上传：`file` + 可选 `path`（相对路径；`FileName()` 会去目录） |
| `GET` | `/api/stream/{id...}` | MP4 流，须支持 Range（用 `http.ServeContent`） |
| `GET` | `/` | 前端静态页 |

上传须**流式**（`MultipartReader`），禁止改回整表 `ParseMultipartForm` 大文件缓冲。目录浏览以文件系统为准，不要引入数据库维护文件夹内容。

## Backend Rules

- 保持零/极少第三方依赖；优先标准库。
- 业务代码放 `internal/`；入口只在 `cmd/videohub`。
- **路径安全**：所有读写限制在 `VIDEO_DIR` 下；拒绝 `..`、隐藏段；上传 `path` 须净化后再 `MkdirAll`。
- 列表可用短缓存；上传成功后必须 `Invalidate()`。
- 流式播放必须走 `http.ServeContent`（或等价 Range 实现）。
- Go 代码 `gofmt`；新增逻辑补 `internal/library` 等单测。
- 修改配置项时同步 `README.md` / `docs/DESIGN.md` / `docker-compose.yml`。

## Frontend Rules

- 保持无构建链：只改 `web/index.html`、`web/app.js`、`web/style.css`。
- 浏览用 `/api/browse` + 面包屑；搜索用 `/api/search`；URL 同步 `path`/`q`/`v`。
- **上传与文件浏览分属独立面板**，不要把上传表单塞进浏览列表中间。
- 上传支持多选文件与文件夹（`webkitdirectory`）；客户端筛选 `.mp4`，逐个 `POST`；带相对路径时用表单字段 `path`。
- 播放器用 `<video controls playsinline>`，`src` 指向 `/api/stream/...`。
- 移动端需可用：保留 `viewport`，布局在窄屏下单列堆叠。
- 用户可见文案可用中文；错误态要明确。

## Security / Boundaries

- **无鉴权**：默认内网使用；不要建议或实现「直接公网裸奔」方案，除非用户明确要求并接受风险。
- Docker 上传需要视频卷**可写**；权限问题优先查 `PUID`/`PGID` 与挂载属主，而不是放宽为长期 root 跑业务。
- 不要引入密钥进仓库；不要提交真实视频大文件（见 `.gitignore`）。

## When Changing Behavior

1. 先读 `docs/DESIGN.md` 对应章节，再改代码。
2. 行为变更同步更新 DESIGN 验收项与 README。
3. 跑通：`go test -buildvcs=false ./...`；涉及上传/权限时用 curl 或 compose 冒烟。
4. 不主动 `git commit` / `push`，除非用户明确要求。

## References

| 文档 | 用途 |
|------|------|
| [README.md](README.md) | 人类快速开始 |
| [docs/DESIGN.md](docs/DESIGN.md) | 架构、API、安全、验收 |
| [agents.md](https://agents.md/) | AGENTS.md 开放格式说明 |
| 结构参考 | [QuantumNous/new-api AGENTS.md](https://github.com/QuantumNous/new-api/blob/main/AGENTS.md)（前后端分区规则） |
| 结构参考 | [nextlevelbuilder/goclaw AGENTS.md](https://github.com/nextlevelbuilder/goclaw/blob/dev/AGENTS.md)（Go + Web UI 技术栈/目录） |
