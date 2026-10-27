# Slate / Backend

单个 Go 二进制 `slate`，负责 Web API、设备协议、动态内容渲染、音频转码与 TTS。生产环境同时托管 `frontend/dist`（SPA fallback）。

- **数据**：PostgreSQL 18；图片和音频是按 sha256 寻址的文件。
- **依赖**：标准库 `net/http`、pgx v5 + sqlc、ffmpeg（只用于音频）。没有 Web 框架和 ORM。

## 命令

| 命令 | 作用 |
| --- | --- |
| `slate serve` | 应用迁移后启动 HTTP 服务和后台任务（默认） |
| `slate migrate` | 只应用迁移 |
| `slate import <mysql-url>` | 把 v0.1.x 的 MySQL 部署导入空库 |

启动时读取当前目录的 `.env`，已有的环境变量优先。日志为 JSON，输出到 stdout。

## 目录

| 目录 | 内容 |
| --- | --- |
| `cmd/slate/` | 入口与子命令 |
| `cmd/gencatalog/` | 生成前端用的 `catalog.json`：音色、热榜源、字体、模板 |
| `cmd/genfont/` | 把 LVGL C 字体或 BDF 字体转成点阵 JSON |
| `internal/api/` | 请求与响应类型，前端类型由此生成 |
| `internal/httpapi/` | 路由、鉴权、限流、handler |
| `internal/store/` | sqlc 查询（`sql/`）与生成代码（`db/`） |
| `internal/dynamic/` | 动态内容：配置校验、数据源、调度、TTS 队列、blob GC |
| `internal/render/` | 1bpp 帧绘制：点阵字体、布局、各类型模板 |
| `internal/hotlist/` | 热榜抓取，`sources.json` 是榜单源清单 |
| `internal/blob/` | sha256 内容寻址文件存储 |
| `internal/media/` | ffmpeg 音频转码 |
| `internal/legacyimport/` | v0.1.x MySQL 导入 |
| `internal/fetch/` | 第三方数据源的 HTTP 客户端 |
| `internal/config/` | 环境变量与 `.env` |
| `migrations/` | 嵌入式 SQL 迁移 |

---

## 数据模型

定义见 [migrations/0001_init.sql](migrations/0001_init.sql)。

- `users` 拥有 `devices` 和 `groups`，`groups` 包含 `contents`，删组时级联删内容。
- 动态内容的配置、数据和调度状态在 `content_dynamic`，音频和 TTS 队列在 `content_audio`。
- `contents.image_hash`、`content_audio.hash` 指向 blob。blob 不可变，内容变化就是换 hash。

**设备**

- 以 `mac` 标识。同一 mac 重新注册视为物理重置：清空归属和所选内容组，轮换 secret 与配对码。
- secret 只在注册响应里出现一次，库里存 sha256。
- 配对码在绑定、解绑后轮换。

**迁移**

- 按文件名顺序执行，记录在 `schema_migrations` 并校验 checksum。
- 已应用的文件不能改，改 schema 就加新文件。
- 多个实例同时启动时由 advisory lock 串行。

---

## API

全部在 `/api` 下，JSON 字段 camelCase，时间为 UTC ISO 8601。响应统一为：

```json
{ "success": true, "message": "操作成功", "data": {} }
```

- 失败时 `success` 为 false，`message` 是可直接展示的中文说明，HTTP 状态码表达错误类型。
- JSON 请求体必须是单个对象，未知字段报错。
- 类型定义在 `internal/api`，前端从生成的 TypeScript 引用。

### 账号

| 接口 | 说明 |
| --- | --- |
| `POST /api/users` | 注册 `{ email, username, password }` |
| `POST /api/sessions` | 登录 `{ identifier, password }`，identifier 为邮箱或用户名 |
| `GET /api/users/me` | 当前用户 |
| `GET /api/health` | 健康检查 |

### Web 管理

以下接口都需要 `Authorization: Bearer <JWT>`。

**设备**

| 接口 | 说明 |
| --- | --- |
| `GET /api/devices` | 设备列表 |
| `PUT /api/devices/order` | 排序 |
| `POST /api/devices/claims` | 用配对码绑定 `{ pairCode }` |
| `GET` `PATCH /api/devices/{id}` | 读取、修改 `{ name?, groupId? }` |
| `DELETE /api/devices/{id}/binding` | 解绑 |

**内容组**

| 接口 | 说明 |
| --- | --- |
| `GET` `POST /api/groups` | 列表、创建 |
| `PUT /api/groups/order` | 排序 |
| `GET` `PATCH` `DELETE /api/groups/{id}` | 读取、修改、删除 |
| `GET` `POST /api/groups/{id}/contents` | 组内内容列表、创建内容 |
| `PUT /api/groups/{id}/contents/order` | 组内内容排序 |

**内容**

| 接口 | 说明 |
| --- | --- |
| `GET` `PATCH` `DELETE /api/contents/{id}` | 读取、修改、删除 |
| `POST /api/contents/{id}/refresh` | 立即刷新动态内容 |
| `POST /api/contents/preview` | 预览未保存的动态配置 |
| `POST /api/contents/{id}/preview` | 用已保存内容的数据预览新配置 |
| `DELETE /api/contents/{id}/audio` | 删除音频 |
| `POST /api/contents/{id}/audio/tts` | 设置 TTS 朗读 |
| `GET /api/blobs/{hash}` | 帧或音频原始字节 |
| `GET /api/weather/cities?q=` | 城市搜索 |

**图片内容**用 `multipart/form-data` 创建和修改，只接受三个字段：

| 字段 | 内容 |
| --- | --- |
| `frame` | 浏览器渲染好的 400×300 packed 1bpp，15000 字节，MSB first，1 为白 |
| `audio` | 任意支持格式的音频，≤ 5 MB |
| `frameName` | 状态栏标题 |

- 其他字段或重复字段直接报错，请求体总大小不超过两个文件上限之和加 64 KB。
- 服务端不处理图片像素，只校验长度。
- 换图片会保留已有音频，删除音频走 `DELETE /audio`。

**动态内容**用 JSON（`CreateDynamicContentRequest` / `PatchContentRequest`）。修改配置时先按新配置取数并渲染，成功后才落库。

### Dashboard 数据推送

```text
POST /api/contents/{id}/data
{ "version": 1, "data": { ... } }
```

- 不需要 JWT，内容 ID 本身就是凭证；泄漏后删除内容重建。
- 请求体 ≤ 64 KB，每个内容每分钟 30 次。

内置模板「AI 限额监控」（`ai_quota_monitor`）展示一个服务的两个限额窗口，Claude Code、Codex 都适用。`data` 字段：

| 字段 | 示例 | 说明 |
| --- | --- | --- |
| `service_label` | `Codex` | 服务名 |
| `plan_label` | `Pro` | 套餐 |
| `status_label` | `正常` | 状态 |
| `primary_window_label` | `5小时` | 第一个窗口名，重置时间标题显示为「5小时重置」 |
| `primary_used_percent` | `42` | 使用率，0–100 |
| `primary_reset_at_label` | `14:00` | 重置时间 |
| `secondary_window_label` | `每周` | 第二个窗口名 |
| `secondary_used_percent` | `18` | 使用率，0–100 |
| `secondary_reset_at_label` | `周四 09:00` | 重置时间 |
| `updated_label` | `10-27 16:10` | 更新时间 |

### 设备协议 v2

| 接口 | 请求体 |
| --- | --- |
| `POST /api/device/register` | `{ protocol, mac }`，无需鉴权 |
| `POST /api/device/sync` | `{ protocol, manifestVersion, telemetry }` |
| `POST /api/device/group` | `{ protocol, action, groupId?, manifestVersion }` |
| `GET /api/device/blobs/{hash}` | — |

**注册**返回 `deviceId`、`secret`（64 位 hex）、`pairCode` 和 `serverTime`（Unix 毫秒）。之后的请求都带 `Authorization: Bearer <secret>`。

**切组**的 `action` 为 `next`、`prev` 或 `select`，`select` 时带 `groupId`。

**`sync` 和 `group`** 返回 `DeviceState`：

- 设备名、是否绑定、配对码；
- 当前组：位置、内容数、`manifestVersion`；
- `manifest`：只在设备上报的版本与服务端不同时附带。

**manifest** 是帧列表，每帧包含 `id`、`seq`、`statusBarText`、`image {hash, size}`、`audio`、`refreshAt`。

- `refreshAt` 是动态帧下次重新渲染的 Unix 秒，静态帧为 null。
- 版本是组 ID、组名和帧列表的 sha256 前 32 位，帧的任何变化都会换版本。

**定时唤醒**时设备在 telemetry 里带上 `currentContentId`。该内容已到期就先同步重新渲染，本轮即可拿到新帧。

**版本**：`protocol` 与 `api.DeviceProtocol` 不一致时返回 426，固件的 `api::kProtocol` 必须同步修改。

### 限流

每分钟固定窗口，单进程内计数。

| 端点 | 限制 |
| --- | --- |
| 注册 / 登录 | 每 IP 5 / 10 次 |
| 设备注册 | 每 IP 20 次 |
| 绑定设备 | 每 IP + 用户 5 次 |
| 城市搜索 | 每 IP 30 次 |
| Dashboard 推送 | 每个内容 30 次 |

直连地址是本机或内网地址（即经过反向代理）时，客户端 IP 才取自 `X-Forwarded-For`。

---

## 存储

`BLOB_DIR` 下的路径是 `sha256/<前两位>/<hash>`，写入用临时文件加 rename。

后台每 6 小时清理一次：没有被引用、且超过 1 小时没被写入的文件会删除。`Put` 遇到已存在的文件会刷新 mtime，所以刚被重新引用的 blob 不会在提交前被清掉。

只应有一个开启后台任务的进程使用同一个 `BLOB_DIR`。

## 动态内容

**配置**：`internal/dynamic/config.go` 校验并补全配置。

**数据源**：在 `providers.go`。同一城市、榜单源的请求在进程内合并，按刷新间隔缓存。

**历史上的今天**有两个数据源：

- **Wikipedia**：取中文维基的 onthisday 接口（简体变体，带标识 Slate 的 User-Agent），再交给 AI 模型挑选最多 5 条并改写成屏幕文案。原文混杂繁简、篇幅过长，不直接上屏，所以这个源必须配置 AI。
- **百度百科**：直接取当天词条，不经过 AI。

两个源的结果都按「数据源 + 日期 + AI 模型」缓存 24 小时。

**渲染**：`internal/render` 直接画 1bpp，不经过浏览器或 SVG。改布局后运行 `go test ./internal/render -update` 重写 golden PNG，并检查图片差异。

**调度**：`BACKGROUND_WORKERS=true` 时运行。

- 调度器用 `FOR UPDATE SKIP LOCKED` 租约认领到期内容，每轮最多 5 个，失败后指数退避。
- 内容新建、修改或渲染后立即重新计算下次唤醒。
- 数据源失败且旧数据仍有效时，继续用旧数据渲染并记录错误。

## 音频

**上传**：ffmpeg 转成 16 kHz 单声道 s16le PCM，截断到 60 秒。ffmpeg 最多并发 2 个，排队超过 8 个返回 429。

**TTS**：调用 OpenAI 兼容的 `/chat/completions`（`audio: { format: 'pcm16', voice }`），把 24 kHz PCM 重采样到 16 kHz。

- 流必须以 `finish_reason: "stop"` 结束才算成功，中途断开按失败重试。
- 任务逐条认领，最多重试 3 次。
- 进程在最后一次尝试中退出的任务，租约过期后标为失败。

---

## 配置

缺少必填项或格式错误时启动直接退出。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | — | 必填，`postgresql://user:pwd@host:5432/db` |
| `JWT_SECRET` | — | 必填，至少 32 字符且熵足够 |
| `JWT_EXPIRATION` | `7d` | 秒数或 `15m` / `1h` / `7d` / `2w` |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `PORT` | `9494` | |
| `BLOB_DIR` | `./blobs` | 镜像内为 `/data/blobs` |
| `PUBLIC_DIR` | `public` | 前端构建产物目录，镜像内为 `/app/public` |
| `BACKGROUND_WORKERS` | `true` | 是否运行动态刷新、TTS 和 blob 清理 |
| `QWEATHER_API_KEY` | — | 天气 |
| `QWEATHER_API_HOST` | — | 天气，带 `https://` |
| `AI_API_KEY` / `AI_BASE_URL` | — | 历史上的今天用 Wikipedia 源时必填 |
| `AI_MODEL` | `gpt-4o-mini` | |
| `TTS_API_KEY` / `TTS_BASE_URL` | — | TTS，可不配 |
| `TTS_MODEL` | `mimo-v2.5-tts` | |

`JWT_SECRET` 可以用 `openssl rand -hex 64` 生成。本地开发的样例见 [.env.example](.env.example)。

---

## 开发

本地启动见[根 README](../README.md#本地开发)，提交前运行 `scripts/check.sh backend`。

**生成代码**：改了 `internal/store/sql`、`migrations` 或 `internal/api` 后运行 `go generate ./...`。生成物（`internal/store/db`、`frontend/src/api/generated.ts`、`catalog.json`）一起提交，CI 会检查它们与源码一致。

**测试**：`go test ./...`。

- 涉及数据库的测试需要 `SLATE_TEST_DATABASE_URL`，没设时跳过。每个测试在该库里建一个独立 schema、结束后删除，只需要建 schema 的权限。
- `internal/media` 的测试需要本机有 ffmpeg，没有时跳过。镜像构建时会用镜像里的 ffmpeg 再跑一遍，缺格式就构建失败。
- 热榜的线上抓取测试要设 `SLATE_TEST_LIVE_HOTLIST=1`。

## Docker

镜像只含 `slate`、前端产物和一个从源码编译的静态 ffmpeg（只开音频解码与重采样）。默认命令是 `serve`，健康检查访问 `/api/health`。

ffmpeg 的版本和源码包 sha256 在 `Dockerfile` 的 `FFMPEG_VERSION` / `FFMPEG_SHA256`，升级时两者一起改。
