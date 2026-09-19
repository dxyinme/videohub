# Video Hub 设计文档

## 1. 目标

构建一个**简单、可 Docker 部署**的本地视频站：前后端统一交付（单进程 / 单镜像），用 Go 提供后端与静态资源服务。

### 1.1 功能范围（MVP）

| 能力 | 说明 |
|------|------|
| 视频浏览 | 群晖风格目录浏览：按路径 `ReadDir` 当前层文件夹/MP4；支持面包屑与全局名称搜索（无数据库，磁盘为真相源） |
| 视频播放 | 浏览器内播放 MP4（HTML5 `<video>`），支持拖动进度条（Range 请求） |
| 视频上传 | 通过页面或 API 上传 `.mp4` 到视频根目录，上传后刷新列表 |
| Docker 部署 | 提供 `Dockerfile` 与示例 `docker-compose.yml`，挂载宿主机视频目录即可用 |

### 1.2 非目标（本期不做）

- 用户登录 / 权限 / 多租户
- 转码、多码率、HLS/DASH
- 封面图自动截取、字幕管理
- 数据库、收藏、播放历史（可后续扩展）

---

## 2. 总体架构

采用 **Go 单体应用 + 内嵌/同目录静态前端**：

```
浏览器
  │  HTTP
  ▼
┌─────────────────────────────────────┐
│  Video Hub (单二进制)                │
│  ┌─────────────┐  ┌───────────────┐ │
│  │ HTTP API    │  │ 静态资源      │ │
│  │ /api/...    │  │ / (HTML/JS)   │ │
│  └──────┬──────┘  └───────────────┘ │
│         │                           │
│  ┌──────▼──────────────────────────┐│
│  │ 视频库 (扫描配置目录 + 安全路径) ││
│  └──────┬──────────────────────────┘│
└─────────┼───────────────────────────┘
          │ 读文件
          ▼
   配置的视频根目录 (volume)
```

设计原则：

1. **前后端统一**：一个进程监听一个端口；前端页面与 API 同源，无独立 Node 服务。
2. **无状态**：列表由每次请求扫描或进程内短缓存得到，不落库。
3. **路径安全**：所有文件访问必须限制在配置的视频根目录内，禁止 `..` 穿越。

---

## 3. 技术选型

| 层 | 选择 | 理由 |
|----|------|------|
| 语言 / 运行时 | Go 1.23+ | 仓库已初始化；静态编译，适合 Docker |
| HTTP | 标准库 `net/http`（或轻量路由如 `chi`） | MVP 路由少，依赖越少越好 |
| 前端 | 单页或少量静态 HTML + 原生 JS（或极简模板） | 无构建链也可工作；若需打包可后续再加 |
| 配置 | 环境变量 + 可选 YAML/JSON 文件 | 便于 Docker `-e` / compose |
| 容器 | 多阶段构建：`golang` 编译 → `distroless`/`alpine` 运行 | 镜像小、只含二进制与静态资源 |

---

## 4. 配置

| 配置项 | 环境变量示例 | 默认值 | 说明 |
|--------|--------------|--------|------|
| 监听地址 | `VIDEOHUB_ADDR` | `:8080` | HTTP 监听 |
| 视频根目录 | `VIDEOHUB_VIDEO_DIR` | `./videos` | 扫描与读取的根路径 |
| 扫描深度 | `VIDEOHUB_SCAN_DEPTH` | `0`（不限）或固定层数 | 可选；MVP 可先递归全部 |
| 缓存 TTL | `VIDEOHUB_LIST_CACHE_SEC` | `5` | 列表短缓存，避免频繁扫盘 |
| 最大上传 | `VIDEOHUB_MAX_UPLOAD_MB` | `2048` | 单个上传文件大小上限（MiB） |

配置在进程启动时加载一次；视频目录变更后刷新列表即可（缓存过期、上传成功会主动失效缓存）。

---

## 5. 后端设计

### 5.1 模块划分

```
cmd/videohub/          # main：加载配置、注册路由、启动服务
internal/config/       # 配置解析与校验
internal/library/      # 扫描视频目录、生成相对路径 ID、安全 Join
internal/api/          # HTTP handlers
web/                   # 前端静态文件 + embed.FS
```

### 5.2 视频发现规则

- 仅收录扩展名（大小写不敏感）：`.mp4`
- 以**相对视频根目录的路径**作为视频 ID（URL 安全编码），例如 `movies/foo.mp4`
- 忽略隐藏文件/目录（可选：以 `.` 开头的条目）
- 扫描失败（目录不存在、无权限）时启动应报错退出或返回明确错误，避免静默空列表难排查

### 5.3 HTTP API

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/` | 前端页面 |
| `GET` | `/api/health` | 健康检查（Docker / 编排用） |
| `GET` | `/api/videos` | 全量视频列表 JSON（兼容/搜索底层） |
| `GET` | `/api/browse` | 当前目录浏览：`?path=`（空=根）；`ReadDir` 返回 folders + videos |
| `GET` | `/api/search` | 全局搜索：`?q=`，最多 200 条 |
| `POST` | `/api/videos` | 上传 MP4（`multipart/form-data`，字段名 `file`） |
| `GET` | `/api/stream/{id...}` | 流式输出 MP4，支持 `Range`（Go ServeMux 要求 `{id...}` 位于路径末尾） |

#### `GET /api/browse` 响应示例

```json
{
  "path": "movies",
  "parent": "",
  "folders": [{ "name": "action", "path": "movies/action" }],
  "videos": [{ "id": "movies/a.mp4", "name": "a.mp4", "path": "movies/a.mp4", "size": 12 }]
}
```

不使用数据库维护目录；空文件夹也会出现在 `folders` 中。

#### `GET /api/videos` 响应示例

```json
{
  "root": "/data/videos",
  "videos": [
    {
      "id": "movies/foo.mp4",
      "name": "foo.mp4",
      "path": "movies/foo.mp4",
      "size": 104857600
    }
  ]
}
```

#### `POST /api/videos`

- `Content-Type: multipart/form-data`
  - 字段 `file`：文件内容（必填）
  - 字段 `path`：可选相对路径（如 `movies/a.mp4`）；因 RFC 7578，`FileName()` 会去掉目录，故用独立字段保留文件夹结构
- 仅接受扩展名为 `.mp4` 的文件；`path`/`filename` 经净化，禁止 `..` 与隐藏段
- 写入视频根目录（可含子目录，自动 `MkdirAll`）；同名时自动追加 `_1`、`_2`…
- 成功：`201` + `{"video": {...}}`；超限：`413`；非法文件名/类型：`400`
- 上传成功后失效列表缓存
- 前端支持多选文件与选择文件夹，自动筛选其中的 `.mp4` 后逐个上传

#### `GET /api/stream/{id...}`

- `Content-Type: video/mp4`
- 必须支持 `Accept-Ranges: bytes` 与 `206 Partial Content`，否则进度条拖动不可用
- 校验 `id` 解码后路径仍在根目录内；越界则 `403`；不存在则 `404`；非文件则 `400`
- 示例：`/api/stream/movies/foo.mp4`

可用 `http.ServeContent` 实现 Range，避免手写 Range 解析。

### 5.4 安全要点

1. `filepath.Clean` + 前缀检查：解析后的绝对路径必须位于 `VIDEO_DIR` 之下。
2. 不暴露绝对路径给客户端（列表里可用相对 path；`root` 字段是否返回可配置，生产可关闭）。
3. 本期无鉴权：默认假设部署在受信任的内网；文档中注明勿直接暴露公网。
4. 上传需限制大小与扩展名；Docker 挂载视频目录时需可写（`:rw`）。

---

## 6. 前端设计

### 6.1 页面结构（单页即可）

1. **播放区**：独立面板，`<video controls>`，`src` 指向 `/api/stream/{id...}`。
2. **文件浏览模块**：面包屑 + 搜索 + 当前目录条目（与上传分离）。
3. **上传模块**：独立面板；多选文件或选文件夹（自动筛选 MP4）；成功后刷新浏览目录。
4. **空态 / 错误态**：空文件夹、无搜索结果或接口失败时给出清晰提示。

### 6.2 交互

- 进入站点默认 `GET /api/browse`（根目录）
- URL：`?path=` / `?q=` / `?v=` 保持目录、搜索与当前片
- 选择视频后更新播放器 `src`（注意自动播放策略）

### 6.3 交付方式

推荐 `embed.FS` 将 `web/` 打进二进制，Docker 只需一个文件 + 挂载视频目录。

---

## 7. Docker 部署

### 7.1 镜像

- 构建阶段：编译 `cmd/videohub`
- 运行阶段：非 root 用户、只读根文件系统（可选）、仅挂载视频卷可写或只读均可（只读更安全）

### 7.2 示例 compose

```yaml
services:
  videohub:
    image: videohub:latest
    ports:
      - "8080:8080"
    environment:
      VIDEOHUB_ADDR: ":8080"
      VIDEOHUB_VIDEO_DIR: "/videos"
    volumes:
      - /path/on/host/videos:/videos:rw
```

访问：`http://localhost:8080`

---

## 8. 目录与仓库布局（目标）

```
videohub/
├── cmd/videohub/main.go
├── internal/
│   ├── config/
│   ├── library/
│   └── api/
├── web/
│   ├── index.html
│   ├── app.js
│   └── style.css
├── Dockerfile
├── docker-compose.yml
├── go.mod
├── README.md
└── docs/DESIGN.md
```

---

## 9. 实现顺序建议

1. 配置加载 + 健康检查 + 静态页占位
2. 视频目录扫描 + `/api/videos`
3. Range 流式播放 + `/api/stream/{id...}`
4. 前端列表与播放器联调
5. Dockerfile / compose 与 README 使用说明

---

## 10. 后续可扩展（不在 MVP）

- 缩略图 / ffprobe 元数据（时长、分辨率）
- Webhook 或 `fsnotify` 监听目录变更
- 简单 Token / Basic Auth
- 支持更多容器格式（仍由浏览器解码能力决定）
- 左侧完整目录树 / 旁路搜索索引（十万级）

---

## 11. 验收标准

- [ ] 根目录与子目录可通过面包屑/进入文件夹浏览；空文件夹可见
- [ ] 搜索可按名称或路径找到视频
- [ ] 点击可在页面内播放，可拖动进度条
- [ ] 可通过页面/API 上传 `.mp4`，上传后出现在对应目录
- [ ] `docker compose up` 后通过映射端口可访问
- [ ] 请求试图访问根目录外的路径时被拒绝（404/403）
- [ ] 不引入数据库维护目录内容
