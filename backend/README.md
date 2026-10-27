# Slate / Backend

单个 Go 二进制 `slate`：Web API、设备协议、动态内容渲染、音频转码与 TTS，生产环境同时托管 `frontend/dist`（SPA fallback）。数据在 PostgreSQL 18，图片和音频是按 sha256 寻址的文件。

依赖：标准库 `net/http`、pgx v5 + sqlc、ffmpeg（只用于音频）。没有 Web 框架和 ORM。

## 命令

```text
slate serve               应用迁移后启动 HTTP 服务和后台任务（默认）
slate migrate             只应用迁移
slate import <mysql-url>  把 v0.1.x 的 MySQL 部署导入空库，见根 README「从 v0.1.x 升级」
slate job <name>          运行 dashboard 推送任务，见下文
slate claude-statusline   Claude Code 的 statusLine 命令，顺带推送限额数据
```

启动时读取当前目录的 `.env`（已有的环境变量优先），日志为 JSON，输出到 stdout。

## 目录

```text
cmd/slate/          入口与子命令
cmd/gencatalog/     生成 frontend/src/api/catalog.json（音色、热榜源、字体、模板）
cmd/genfont/        把 LVGL C 字体或 BDF 字体转成 render 用的点阵 JSON，由 scripts/fonts/backend.sh 调用
internal/
├── api/            请求与响应类型，tygo 由此生成 frontend/src/api/generated.ts
├── httpapi/        路由、鉴权、限流、handler
├── store/          sqlc 查询（sql/）与生成代码（db/）
├── dynamic/        动态内容：配置校验、数据源、调度、TTS 队列、blob GC
├── render/         1bpp 帧绘制：点阵字体、布局、各类型模板；testdata/frames 是 golden PNG
├── hotlist/        热榜抓取，sources.json 是榜单源清单
├── blob/           sha256 内容寻址文件存储
├── media/          ffmpeg 音频转码
├── job/            dashboard 推送任务
├── legacyimport/   v0.1.x MySQL 导入
├── fetch/          第三方数据源的 HTTP 客户端
└── config/         环境变量与 .env
migrations/         嵌入式 SQL 迁移
```

## 数据模型

定义见 [migrations/0001_init.sql](migrations/0001_init.sql)。`users` 拥有 `devices` 和 `groups`，`groups` 包含 `contents`（删组级联删内容）。动态内容的配置、数据和调度状态在 `content_dynamic`，音频和 TTS 队列在 `content_audio`。

- 设备以 `mac` 标识。同一 mac 重新注册视为物理重置：清空归属和所选内容组，轮换 secret 与配对码。
- 设备 secret 只在注册响应里出现一次，库里存 sha256。配对码在绑定、解绑后轮换。
- `contents.image_hash`、`content_audio.hash` 指向 blob；blob 不可变，内容变化就是换 hash。
- 迁移按文件名顺序执行，记录在 `schema_migrations` 并校验 checksum：已应用的文件不能改，改 schema 加新文件。多个实例同时启动时由 advisory lock 串行。

## API

全部在 `/api` 下，JSON 字段 camelCase，时间为 UTC ISO 8601。响应统一为：

```json
{ "success": true, "message": "操作成功", "data": {} }
```

失败时 `success` 为 false，`message` 是可直接展示的中文说明，HTTP 状态码表达错误类型。JSON 请求体必须是单个对象，未知字段报错。类型定义在 `internal/api`，前端从生成的 TypeScript 引用。

**账号**

```text
POST   /api/users                  注册 { email, username, password }
POST   /api/sessions               登录 { identifier, password }，identifier 为邮箱或用户名
GET    /api/users/me
GET    /api/health
```

**Web 管理**（`Authorization: Bearer <JWT>`）

```text
GET    /api/devices                PUT  /api/devices/order
POST   /api/devices/claims         用配对码绑定 { pairCode }
GET    /api/devices/{id}           PATCH /api/devices/{id}   { name?, groupId? }
DELETE /api/devices/{id}/binding

GET    /api/groups                 POST /api/groups          PUT /api/groups/order
GET    /api/groups/{id}            PATCH /api/groups/{id}    DELETE /api/groups/{id}
GET    /api/groups/{id}/contents   POST /api/groups/{id}/contents
PUT    /api/groups/{id}/contents/order

GET    /api/contents/{id}          PATCH /api/contents/{id}  DELETE /api/contents/{id}
POST   /api/contents/{id}/refresh  立即刷新动态内容
POST   /api/contents/preview       预览未保存的动态配置
POST   /api/contents/{id}/preview  用已保存内容的数据预览新配置
DELETE /api/contents/{id}/audio
POST   /api/contents/{id}/audio/tts
GET    /api/blobs/{hash}           帧或音频原始字节
GET    /api/weather/cities?q=
```

图片内容用 `multipart/form-data` 创建和修改：`frame` 是浏览器渲染好的 400×300 packed 1bpp（15000 字节，MSB first，1 为白），`audio` 是任意支持格式的音频（≤ 5 MB），`frameName` 是状态栏标题。服务端不处理图片像素，只校验长度。换图片会保留已有音频，删除音频走 `DELETE /audio`。动态内容用 JSON（`CreateDynamicContentRequest` / `PatchContentRequest`）；修改配置时先按新配置取数并渲染，成功后才落库。

**Dashboard 数据推送**

```text
POST   /api/contents/{id}/data     { "version": 1, "data": { ... } }
```

不需要 JWT，内容 ID 本身就是凭证，泄漏后删除内容重建。请求体 ≤ 64 KB，每个内容每分钟 30 次。

**设备协议 v2**

```text
POST   /api/device/register        { protocol, mac }，无需鉴权
POST   /api/device/sync            { protocol, manifestVersion, telemetry }
POST   /api/device/group           { protocol, action: next|prev|select, groupId?, manifestVersion }
GET    /api/device/blobs/{hash}
```

- 注册返回 `deviceId`、`secret`（64 位 hex）、`pairCode` 和 `serverTime`（Unix 毫秒），之后的请求都带 `Authorization: Bearer <secret>`。
- `sync` 和 `group` 返回 `DeviceState`：设备名、是否绑定、配对码、当前组（位置、内容数、`manifestVersion`），以及只在设备上报的版本与服务端不同时才附带的 `manifest`。
- manifest 是帧列表：`id`、`seq`、`statusBarText`、`image {hash, size}`、`audio`、`refreshAt`（动态帧下次重新渲染的 Unix 秒，静态帧为 null）。版本是组 ID、组名和帧列表的 sha256 前 32 位，帧的任何变化都会换版本。
- 定时唤醒时设备在 telemetry 里带上 `currentContentId`，该内容已到期就先同步重新渲染，本轮即可拿到新帧。
- `protocol` 与 `api.DeviceProtocol` 不一致时返回 426；固件的 `api::kProtocol` 必须同步修改。

**限流**（每分钟固定窗口，单进程内计数）

| 端点 | 限制 |
| --- | --- |
| 注册 / 登录 | 每 IP 5 / 10 次 |
| 设备注册 | 每 IP 20 次 |
| 绑定设备 | 每 IP + 用户 5 次 |
| 城市搜索 | 每 IP 30 次 |
| Dashboard 推送 | 每个内容 30 次 |

直连地址是本机或内网地址（即经过反向代理）时，客户端 IP 才取自 `X-Forwarded-For`。

## 存储

`BLOB_DIR` 下是 `sha256/<前两位>/<hash>`，写入用临时文件加 rename。后台每 6 小时清理没有被引用、且超过 1 小时没被写入的文件；`Put` 遇到已存在的文件会刷新 mtime，所以刚被重新引用的 blob 不会在提交前被清掉。只应有一个开启后台任务的进程使用同一个 `BLOB_DIR`。

## 动态内容与音频

- **类型与配置**：`internal/dynamic/config.go` 校验并补全配置；数据源在 `providers.go`，同一城市、榜单源的请求在进程内合并并按刷新间隔缓存。
- **渲染**：`internal/render` 直接画 1bpp，不经过浏览器或 SVG。改布局后运行 `go test ./internal/render -update` 重写 golden PNG，并检查图片差异。
- **调度**：`BACKGROUND_WORKERS=true` 时，调度器用 `FOR UPDATE SKIP LOCKED` 租约认领到期内容，每轮最多 5 个，失败后指数退避；内容新建、修改或渲染后会立即重新计算下次唤醒。数据源失败且旧数据仍有效时继续用旧数据渲染，并记录错误。
- **音频**：上传的音频由 ffmpeg 转成 16 kHz 单声道 s16le PCM，截断到 60 秒；ffmpeg 最多并发 2 个，排队超过 8 个返回 429。TTS 调用 OpenAI 兼容的 `/chat/completions`（`audio: { format: 'pcm16', voice }`），把 24 kHz PCM 重采样到 16 kHz。TTS 任务逐条认领，最多重试 3 次；进程在最后一次尝试中退出的任务会在租约过期后标为失败。

## 配置

缺少必填项或格式错误时启动直接退出。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | — | 必填，`postgresql://user:pwd@host:5432/db` |
| `JWT_SECRET` | — | 必填，至少 32 字符且熵足够，用 `openssl rand -hex 64` 生成 |
| `JWT_EXPIRATION` | `7d` | 秒数或 `15m` / `1h` / `7d` / `2w` |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `PORT` | `9494` | |
| `BLOB_DIR` | `./blobs` | 镜像内为 `/data/blobs` |
| `PUBLIC_DIR` | `public` | 前端构建产物目录，镜像内为 `/app/public` |
| `BACKGROUND_WORKERS` | `true` | 是否运行动态刷新、TTS 和 blob 清理 |
| `QWEATHER_API_KEY` / `QWEATHER_API_HOST` | — | 天气；Host 带 `https://` |
| `AI_API_KEY` / `AI_BASE_URL` / `AI_MODEL` | — / — / `gpt-4o-mini` | 历史上的今天 AI 优化，可不配 |
| `TTS_API_KEY` / `TTS_BASE_URL` / `TTS_MODEL` | — / — / `mimo-v2.5-tts` | TTS，可不配 |

本地开发的样例见 [.env.example](.env.example)。

## Dashboard 推送任务

`slate job <name>` 每隔 `SLATE_JOB_INTERVAL_SECONDS`（默认 600）秒推送一次，`SLATE_JOB_RUN_ONCE=1` 时只跑一次。通用变量：`SLATE_API_BASE`（Slate 地址，如 `http://slate:9494`）、`SLATE_JOB_TIME_ZONE`（默认 `Asia/Shanghai`）。

| 任务 | 必填变量 |
| --- | --- |
| `sub2api-usage-stats` | `SUB2API_BASE`、`SUB2API_EMAIL`、`SUB2API_PASSWORD`、`SUB2API_CONTENT_ID` |
| `claude-code-quota-monitor` | `CLAUDE_QUOTA_CONTENT_ID`；token 取自 `ANTHROPIC_API_KEY` 或 `~/.claude/.credentials.json`，可选 `ANTHROPIC_API_BASE`、`CLAUDE_PLAN_LABEL` |

用同一个镜像作为 compose sidecar 运行时，它不监听端口，需要关掉镜像自带的健康检查：

```yaml
  sub2api-stats:
    image: ghcr.io/qiujun8023/slate:latest
    restart: unless-stopped
    command: ['job', 'sub2api-usage-stats']
    environment:
      SLATE_API_BASE: http://slate:9494
      SUB2API_BASE: https://sub2api.example.com
      SUB2API_EMAIL: ${SUB2API_EMAIL}
      SUB2API_PASSWORD: ${SUB2API_PASSWORD}
      SUB2API_CONTENT_ID: <dashboard 内容 ID>
    healthcheck:
      disable: true
```

`slate claude-statusline` 可以直接作为 Claude Code 的 statusLine 命令：输出限额状态行，并按 `CLAUDE_QUOTA_PUSH_INTERVAL_MS`（默认 60000）在后台推送。

## 开发

本地启动见[根 README](../README.md#本地开发)。

- **生成代码**：改了 `internal/store/sql`、`migrations` 或 `internal/api` 后运行 `go generate ./...`，生成物（`internal/store/db`、`frontend/src/api/generated.ts`、`catalog.json`）一起提交，CI 会检查它们与源码一致。
- **测试**：`go test ./...`。涉及数据库的测试（设备协议、归属、动态内容修改、TTS 回收）需要 `SLATE_TEST_DATABASE_URL`，每个测试在该库里建一个独立 schema、结束后删除，只需要建 schema 的权限；没设时跳过。`internal/media` 的测试需要本机有 ffmpeg，没有时跳过；镜像构建时会用镜像里的 ffmpeg 跑一遍，缺格式就构建失败。热榜的线上抓取测试要设 `SLATE_TEST_LIVE_HOTLIST=1`。
- **检查**：仓库根目录的 `scripts/check.sh backend`。

## Docker

镜像只含 `slate`、前端产物和一个从源码编译的静态 ffmpeg（只开音频解码与重采样）。默认命令是 `serve`；健康检查访问 `/api/health`。ffmpeg 的版本和源码包 sha256 在 `Dockerfile` 的 `FFMPEG_VERSION` / `FFMPEG_SHA256`，升级时两者一起改。
