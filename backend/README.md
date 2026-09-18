# Slate / Backend

Bun 直接运行 TypeScript 的 NestJS 11（Fastify 5）服务，数据库为 MySQL 8（Prisma 7 + `@prisma/adapter-mariadb`）。单进程负责：

- Web 管理 API（JWT）
- 设备协议（`Authorization: Bearer <device_secret>`）
- 图片、动态帧、音频渲染与下发（ETag / 304）
- 生产环境同域托管 `frontend/dist`，含 SPA fallback

其他依赖：zod 4 校验、pino 日志、sharp 处理图片、ffmpeg 转码音频、bcryptjs + jsonwebtoken 做账号认证。

## 目录

```text
src/
├── main.ts          启动、/api/v1 前缀、multipart、静态托管
├── common/          错误体系、guard、限流、HTTP / worker 工具
├── infra/           配置、Prisma、blob 存储、JWT、日志
└── modules/
    ├── auth / users          注册、登录
    ├── devices               Web 设备管理 + 固件协议
    ├── groups / contents     内容组、内容、manifest、二进制下发
    ├── dynamic-content       动态内容 provider、调度、布局与渲染、dashboard 推送
    ├── hot-list              热榜抓取
    ├── image-renderer        图片 1bpp 管线与渲染缓存
    ├── audio / tts / ai      ffmpeg 转码、TTS、历史事件 AI 优化
    └── health                /healthz
assets/              位图字库、天气图标、示例图片
prisma/              schema 与 migrations
scripts/             job sidecar、维护与调试脚本，见 scripts/README.md
```

## 数据模型

定义见 [prisma/schema.prisma](prisma/schema.prisma)。`User` 拥有 `Device` 和 `Group`，`Group` 包含多个 `Content`（删除组会级联删除内容）。

- `Device.mac` 是物理设备标识。同一 mac 重新注册视为物理重置：清空归属和所选内容组，轮换 secret 与配对码。
- `device_secret` 只在注册响应里返回一次，数据库只存其 sha256。
- `pair_code` 在绑定、解绑后都会轮换。
- `Group.structure_etag` 反映组结构变化，`manifest_etag` 反映 manifest 任意变化。
- `Content.content_etag` 汇总当前帧的图片、音频、标题等，设备据此判断是否需要刷新。

## API

除 `/healthz` 外都在 `/api/v1` 下。请求与响应的 schema 定义在 [shared/src/types](../shared/src/types)。

**公开**

```text
POST   /users                         注册 { email, username, password }
POST   /sessions                      登录 { identifier, password }，identifier 为邮箱或用户名
GET    /healthz
```

**Web 管理（JWT）**

```text
GET    /users/current
DELETE /sessions/current

GET    /devices
PUT    /devices/order
POST   /devices/claims                用配对码绑定设备
GET    /devices/:id
PATCH  /devices/:id
DELETE /devices/:id/binding

GET    /groups
POST   /groups
PUT    /groups/order
GET    /groups/:groupId
PATCH  /groups/:groupId
DELETE /groups/:groupId

POST   /groups/:groupId/contents
PUT    /groups/:groupId/contents/order
PATCH  /contents/:contentId
DELETE /contents/:contentId
DELETE /contents/:contentId/audio
POST   /contents/:contentId/audio/tts
POST   /contents/:contentId/refresh      立即刷新动态内容
POST   /contents/preview                 预览未保存的动态配置
POST   /contents/:contentId/preview
GET    /dynamic/weather/cities?q=
```

创建和修改内容时：图片内容用 `multipart/form-data`（字段 `image`、`audio`、`threshold`、`mode`、`frame_name`），动态内容用 JSON（`CreateDynamicContentRequest` / `PatchDynamicContentRequest`）。

**内容读取（JWT 或 device secret）**

```text
GET    /groups/:groupId/contents
GET    /groups/:groupId/manifest
GET    /contents/:contentId
GET    /contents/:contentId/image        400×300 packed 1bpp，15000 字节
GET    /contents/:contentId/audio        16 kHz 单声道 s16le PCM
```

Web 预览和设备同步共用这组端点。manifest、image、audio 支持 `If-None-Match`，命中返回 304。

**设备协议**

```text
POST   /devices                          注册 { mac }，无需鉴权
POST   /devices/current/poll             上报 telemetry，返回 DeviceState
PUT    /devices/current/group            切换到指定内容组
POST   /devices/current/group/next
POST   /devices/current/group/prev
```

注册返回 `device_secret`（64 位 hex）和 6 位 `pair_code`，之后的请求都带 `Authorization: Bearer <device_secret>`。当设备因定时器唤醒、当前帧需要刷新且 manifest 未变时，`DeviceState` 会带上 `current_content`，固件只需刷新这一帧。

**Dashboard 数据推送**

```text
POST   /contents/:contentId/data
```

```json
{ "version": 1, "data": { "service_label": "Claude Code", "primary_used_percent": 68 } }
```

不需要 JWT，`contentId` 本身就是凭证，泄漏后删除内容重建。请求体上限 64 KB。模板保存在内容配置里，这个接口只接收数据。

### 鉴权与限流

全局 `JwtAuthGuard` 默认要求 JWT；公开端点用 `@Public()` 标注，再按需加局部 guard。限流为每分钟固定窗口：

| 端点 | 鉴权 | 限流 |
| --- | --- | --- |
| 注册 / 登录 | 公开 | 每 IP 5 / 10 次 |
| 设备注册 | 公开 | 每 IP 20 次 |
| 设备协议 | `DeviceAuthGuard` | — |
| 内容读取 | `JwtOrDeviceAuthGuard` | — |
| 绑定设备 | JWT | 每 IP + 用户 5 次 |
| 城市搜索 | JWT | 每 IP 30 次 |
| Dashboard 推送 | 公开 | 每个 contentId 30 次 |

## 渲染与存储

文件存放在 `BLOB_DIR`（Docker 中为 `/data/blobs`）：

```text
{groupId}/{contentId}.img                 1bpp 帧
{groupId}/{contentId}.{audioEtag}.pcm     音频
image-render-cache/{xx}/{key}.bin         图片渲染缓存
```

写入采用临时文件加 rename，启动时清理超过 24 小时的 `.tmp`。

**图片**：sharp 解码并铺白底 → 以 `contain` 缩放到 400×300 → 灰度 → shared 的 `autoInvert`、`autoContrast`、`ditherTo1bpp`。步骤与前端预览一致，见 [shared/README.md](../shared/README.md#图像处理)。

**动态内容**：类型定义在 `shared/src/dynamic/config.ts`，provider 注册在 `dynamic-content-registry.ts`。帧直接用位图字体绘制成 1bpp，不经过浏览器或 SVG。`BACKGROUND_WORKERS=true` 时，调度器每轮最多处理 5 个到期任务，失败后指数退避。`dynamic_next_run_at` 是内容应更新的时间，`dynamic_refresh_due_at` 会提前一段时间以便准时出帧；manifest 下发 `next_wake_sec` 供固件设置定时唤醒。

**音频**：上传的音频不超过 5 MB，由 ffmpeg 转成 16 kHz 单声道 s16le PCM，最长 60 秒。ffmpeg 最多并发 2 个，排队满 8 个时返回 429。TTS 调用 OpenAI 兼容的 `/chat/completions`（`audio: { format: 'pcm16', voice }`），解析 SSE 中的 PCM，再从 24 kHz 重采样到 16 kHz。

## 环境变量

启动时由 `EnvSchema` 校验，缺少必填项或格式错误会直接退出。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | — | 必填，`mysql://user:pwd@host:3306/db` |
| `JWT_SECRET` | — | 必填，至少 32 字符且熵足够 |
| `JWT_EXPIRATION` | `7d` | 秒数或 `15m` / `1h` / `7d` |
| `DB_ALLOW_PUBLIC_KEY_RETRIEVAL` | `true` | 允许向 MySQL 获取 RSA 公钥，无 TLS 时 `caching_sha2_password` 认证需要 |
| `NODE_ENV` | `development` | `development` / `production` / `test` |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `PORT` | `9494` | |
| `BLOB_DIR` | `./blobs` | |
| `BACKGROUND_WORKERS` | `true` | 是否运行动态刷新与 TTS 后台任务 |
| `QWEATHER_API_KEY` / `QWEATHER_API_HOST` | — | 天气；Host 需带 `https://` |
| `AI_API_KEY` / `AI_BASE_URL` / `AI_MODEL` | — / — / `gpt-4o-mini` | 历史上的今天 AI 优化 |
| `TTS_API_KEY` / `TTS_BASE_URL` / `TTS_MODEL` | — / — / `mimo-v2.5-tts` | TTS |
| `TTS_DEFAULT_VOICE` | `冰糖` | 需在 shared 的 `TTS_VOICES` 中 |

示例见 [.env.example](.env.example)。

## 开发

本地启动见[根 README](../README.md#本地开发)。`bun run dev` 会先执行 `prisma generate` 和 `prisma migrate deploy`；修改 schema 后用 `bun run prisma:migrate` 生成新的 migration。

```bash
bun run --cwd backend test
bun run --cwd backend typecheck
bun run --cwd backend lint
```

## Docker

镜像由 `SLATE_RUN_MODE` 选择运行模式：

- `server`（默认）：执行 `prisma migrate deploy` 后启动服务，并托管 `/app/frontend/dist`。
- `job`：运行 `scripts/jobs/<SLATE_JOB>.ts`，作为 dashboard 数据推送 sidecar，见 [scripts/README.md](scripts/README.md)。
