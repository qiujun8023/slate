# Slate / Backend

后端是 Bun 运行的 NestJS 11（Fastify）服务，使用 Prisma 7 + MySQL 8。单进程承担四类工作：

1. Web 管理 API：给 frontend 使用，JWT 鉴权。
2. 设备协议：给 firmware 使用，`Authorization: Bearer <device_secret>` 鉴权。
3. 内容渲染与资源下发：图片、动态帧、音频、manifest，支持 ETag / 304。
4. 生产静态托管：同镜像内托管 frontend `dist/`，并提供 SPA fallback。

## 技术栈

| 层 | 选型 |
| --- | --- |
| Runtime | Bun 1.x，直接 `bun run src/main.ts` 运行 TypeScript |
| HTTP | NestJS 11 + Fastify 5 + `@nestjs/platform-fastify` |
| ORM | Prisma 7 + `@prisma/adapter-mariadb`，直连 MySQL |
| 校验 | zod 4 + 全局 `ZodValidationPipe` |
| 日志 | nestjs-pino + pino |
| 图像 | sharp 0.34 + shared 1bpp dither |
| 音频 | ffmpeg 转 16 kHz mono s16le PCM |
| 认证 | bcryptjs 密码哈希 + jsonwebtoken JWT |

## 目录

```text
backend/
├── src/
│   ├── main.ts                  Fastify bootstrap、/api/v1 前缀、multipart、static dist、SPA fallback
│   ├── app.module.ts            全局 filter / guard / pipe / interceptor 与业务模块装配
│   ├── common/
│   │   ├── db/                  行锁与批量 sort_order 更新 helper
│   │   ├── errors/              AppError 体系与 Prisma error map
│   │   ├── http/                fetch timeout、SSE parser、client IP
│   │   ├── nest/                @Public/@JsonBody、JWT/device guards、filter、request id、zod pipe
│   │   ├── rate-limit/          固定窗口限速
│   │   ├── utils/               cache、ETag、HTML 文本、intl、value helper
│   │   └── worker/              keyed promise queue、worker loop
│   ├── infra/
│   │   ├── auth/                JWT token service、device secret auth cache
│   │   ├── assets/              运行时资产路径定位
│   │   ├── blob/                image/audio blob 原子写入与清理
│   │   ├── config/              env schema 与 AppConfig
│   │   ├── logger/              pino 配置
│   │   └── prisma/              PrismaService + MariaDB adapter
│   └── modules/
│       ├── ai/                  OpenAI-compatible chat completions，用于历史事件优化
│       ├── audio/               ffmpeg 音频转码与重采样
│       ├── auth/                注册、登录、当前用户
│       ├── contents/            内容 CRUD、manifest、binary、preview、dashboard ingest
│       ├── devices/             Web 设备管理 + 固件设备协议
│       ├── dynamic-content/     动态内容 provider、调度、layout engine、渲染入口
│       ├── groups/              内容组 CRUD、排序、设备切组
│       ├── health/              /healthz
│       ├── hot-list/            热榜抓取与源注册
│       ├── image-renderer/      sharp 图片管线 + 渲染缓存
│       ├── tts/                 OpenAI-compatible TTS
│       └── users/               用户表操作
├── assets/
│   ├── fonts/bitmap-1bpp/       动态帧运行时位图字库
│   ├── fonts/vector/            字体源资产
│   ├── icons/qweather/          天气图标
│   └── vehicles/                示例脚本资产
├── prisma/
│   ├── schema.prisma
│   └── migrations/
└── scripts/
    ├── jobs/                    可选临时 dashboard 推送 sidecar job
    ├── maintenance/             一次性创建或修正内容组
    ├── fonts/                   位图字体提取和生成工具
    ├── debug/                   本地渲染调试
    ├── helpers/                 Nest bootstrap、脚本日志 helper
    └── lib/                     job 共享 env / HTTP / Slate ingest helper
```

## 数据模型

核心模型在 [prisma/schema.prisma](prisma/schema.prisma)：

```text
User
  id, email(unique), username(unique), password(bcrypt)
  └─ owns Device / Group

Device
  mac(unique), secret_hash(unique), pair_code(unique)
  owner_user_id?, selected_group_id?, sort_order
  last_registered_at?, last_seen_at?, battery_pct?, rssi_dbm?, fw_version?

Group
  name, owner_user_id?, sort_order
  structure_etag, manifest_etag
  └─ contents cascade delete

Content
  group_id, sort_order, frame_name?
  kind = image | dynamic
  image_etag, image_size, content_etag
  audio_etag?, audio_size?, audio_status, audio_source?, audio_voice?, audio_text?
  dynamic_type?, dynamic_config?, dynamic_data?
  dynamic_last_run_at?, dynamic_next_run_at?, dynamic_refresh_due_at?
  lease / retry 字段用于 TTS 与动态刷新 worker
```

关键语义：

- `Device.mac` 是物理设备锚点。同 mac 再注册会走 reset 语义：清 owner、清 selected group、轮换 secret 和 pair code。
- `device_secret` 只在注册响应返回一次，数据库只存 `sha256(secret)`。
- `pair_code` 用于 Web claim；绑定或解绑后都会轮换，避免截图复用。
- `Group.structure_etag` 反映组结构变化，`manifest_etag` 反映完整 manifest 变化。
- `Content.content_etag` 是设备当前帧快速刷新用摘要；图片、音频、标题、动态类型和动态数据变化都会影响相关 etag。

## API

除 `/healthz` 外，所有端点都在 `/api/v1` 下。

### 公开端点

```text
POST /api/v1/users
POST /api/v1/sessions
GET  /healthz
```

注册 body：

```json
{ "email": "you@example.com", "username": "you", "password": "password123" }
```

登录 body：

```json
{ "identifier": "you@example.com", "password": "password123" }
```

`identifier` 支持邮箱或用户名。

### Web 管理端点（JWT）

```text
GET    /api/v1/users/current
DELETE /api/v1/sessions/current

GET    /api/v1/devices
PUT    /api/v1/devices/order
POST   /api/v1/devices/claims
GET    /api/v1/devices/:id
PATCH  /api/v1/devices/:id
DELETE /api/v1/devices/:id/binding

GET    /api/v1/groups
POST   /api/v1/groups
PUT    /api/v1/groups/order
GET    /api/v1/groups/:groupId
PATCH  /api/v1/groups/:groupId
DELETE /api/v1/groups/:groupId

POST   /api/v1/groups/:groupId/contents
PUT    /api/v1/groups/:groupId/contents/order
PATCH  /api/v1/contents/:contentId
DELETE /api/v1/contents/:contentId
DELETE /api/v1/contents/:contentId/audio
POST   /api/v1/contents/:contentId/audio/tts
POST   /api/v1/contents/:contentId/refresh
POST   /api/v1/contents/preview
POST   /api/v1/contents/:contentId/preview

GET    /api/v1/dynamic/weather/cities?q=北京
```

`POST /groups/:groupId/contents` 和 `PATCH /contents/:contentId` 支持两种 content type：

- `multipart/form-data`：图片内容。字段包括 `image`、`audio`、`threshold`、`mode`、`frame_name`。
- `application/json`：动态内容。body 符合 `shared` 的 `CreateDynamicContentRequest` 或 `PatchDynamicContentRequest`。

### 内容读取与设备资源下发（JWT 或 device secret）

```text
GET /api/v1/groups/:groupId/contents
GET /api/v1/groups/:groupId/manifest
GET /api/v1/contents/:contentId
GET /api/v1/contents/:contentId/image
GET /api/v1/contents/:contentId/audio
```

这些端点由 `JwtOrDeviceAuthGuard` 保护，Web 预览和固件同步共用同一套读取路径。manifest、image、audio 都支持 ETag；ETag 命中时返回 304。

manifest response：

```ts
{
  group: {
    id: string;
    structure_etag: string;
    manifest_etag: string;
    name: string;
    sort_order: number;
    position: { current: number; total: number };
  };
  contents: Array<{
    id: string;
    seq: number;
    content_etag: string;
    frame_name: string | null;
    device_status_bar_text: string;
    image_etag: string;
    audio_etag: string | null;
    image_size: number;
    audio_size: number | null;
    audio_status: 'none' | 'pending' | 'generating' | 'ready' | 'failed';
    audio_source: 'upload' | 'tts' | null;
    audio_voice: string | null;
    kind: 'image' | 'dynamic';
    dynamic_type: string | null;
    next_wake_sec: number | null;
  }>;
}
```

### Dashboard 外部数据推送

```text
POST /api/v1/contents/:contentId/data
```

该端点只用于 `dashboard` 动态内容，不需要 JWT。`contentId` 本身是 capability URL 凭证；拿到 URL 就能推送。保护措施是 body limit 64 KB 与 `30 req/min/contentId` 固定窗口限速。泄漏后应删除内容重建。

body：

```json
{
  "version": 1,
  "data": {
    "service_label": "Claude Code",
    "primary_used_percent": 68,
    "primary_reset_at_label": "05-27 20:00"
  }
}
```

模板保存在内容配置里；推送接口只接收数据。

### 设备协议

注册端点无鉴权：

```text
POST /api/v1/devices
```

body：

```json
{ "mac": "AA:BB:CC:DD:EE:FF" }
```

响应：

```ts
{
  id: string;
  mac: string;
  device_secret: string; // 64 hex
  pair_code: string;     // 6 chars
  reclaimed: boolean;
  server_time: string;
}
```

后续设备端点使用 `Authorization: Bearer <device_secret>`：

```text
POST /api/v1/devices/current/poll
PUT  /api/v1/devices/current/group
POST /api/v1/devices/current/group/next
POST /api/v1/devices/current/group/prev
```

poll body 可选：

```json
{
  "telemetry": {
    "battery_pct": 85,
    "rssi_dbm": -56,
    "fw_version": "0.1.0",
    "wake_reason": "timer",
    "current_group": "gid",
    "current_content_seq": 3,
    "current_content_etag": "etag",
    "manifest_etag": "etag"
  }
}
```

`DeviceState`：

```ts
{
  device: {
    id: string;
    mac: string;
    name: string | null;
    bound: boolean;
    pair_code: string | null;
    server_time: string;
  };
  group: {
    id: string;
    structure_etag: string;
    manifest_etag: string;
    name: string;
    content_count: number;
    sort_order: number;
    position: { current: number; total: number };
  } | null;
  current_content?: ContentSummary | null;
}
```

`current_content` 只在设备上报 timer wake 且当前帧确实需要刷新、并且无需完整 manifest 同步时返回，用于固件低功耗增量刷新当前帧。

## 鉴权矩阵

`JwtAuthGuard` 和 `RateLimitGuard` 是全局 guard。默认所有端点都要求 JWT；例外由 `@Public()` 和局部 guard 实现，限流由 `@RateLimit(...)` 元数据启用：

| 端点类型 | guard |
| --- | --- |
| Web 管理 | 全局 `JwtAuthGuard` |
| 注册、登录 | `@Public()` + `@RateLimit(authRateLimit)` |
| 设备注册 | `@Public()` + `@RateLimit(deviceRegisterRateLimit)` |
| 设备 current 协议 | `@Public()` + `DeviceAuthGuard` |
| 内容读取 / binary / manifest | `@Public()` + `JwtOrDeviceAuthGuard` |
| dashboard ingest | `@Public()` + `@RateLimit(ingestRateLimit)` + `IngestPayloadSizeGuard` |
| `/healthz` | `@Public()` |

device secret 必须是 64 字符 hex bearer token；JWT 与 device secret 不会互相误判。

## 渲染与存储

### Blob

默认 `BLOB_DIR=./blobs`，Docker 中为 `/data/blobs`。

```text
{BLOB_DIR}/
├── {groupId}/{contentId}.img                   400 x 300 1bpp packed，15000 bytes
├── {groupId}/{contentId}.{audioEtag}.pcm       16 kHz mono s16le raw PCM
└── image-render-cache/{xx}/{key}.bin
```

写入使用临时文件 + rename，按 blob key 串行化，启动时清理 24 小时以上的 `.tmp`。

### 图片内容

`ImageRendererService` 管线：

1. sharp 解码，白底 flatten。
2. resize 到 400 x 300，默认 `contain` letterbox。
3. grayscale raw。
4. `shared.autoInvert` 四角判断黑底反相。
5. `shared.autoContrast(cutoff=1)`。
6. `shared.ditherTo1bpp(mode, threshold)` 输出 packed 1bpp。

输出格式与固件一致：MSB-first，bit=1 白，bit=0 黑。

### 动态内容

动态类型在 `shared/src/dynamic/config.ts` 定义，在 `dynamic-content-registry.ts` 注册 provider 与 JSON definition。当前类型：

- `daily_calendar`
- `month_calendar`
- `weather`
- `history_today`
- `weather_alert`
- `earthquake_report`
- `dashboard`
- `font_test`
- `hot_list`

动态帧由 `DynamicFrameRendererService` 直接用 bitmap canvas 和位图字体绘制成 1bpp，不走浏览器或 SVG。`DynamicContentSchedulerService` 在 `BACKGROUND_WORKERS=true` 时启动，每次最多 claim 5 个到期任务，失败后指数退避。

刷新时间语义：

- provider 计算数据，renderer 写 image blob 和 etag。
- `dynamic_next_run_at` 是下次内容应更新的目标时间。
- `dynamic_refresh_due_at` 是 worker 提前刷新时间，通常会预留 lead time。
- manifest 下发 `next_wake_sec`，固件据此配置 RTC timer。

### 音频与 TTS

上传音频经 ffmpeg 转成：

```text
16000 Hz, mono, signed 16-bit little-endian raw PCM
```

限制：

- 上传音频最大 5 MB。
- 输出最长 60 秒。
- ffmpeg 并发 2，队列上限为并发的 4 倍，繁忙时返回 429。

TTS 使用 OpenAI-compatible `/chat/completions`，请求 `audio: { format: 'pcm16', voice }` 并解析 SSE 中的 base64 PCM，源采样率按 24 kHz 处理，再重采样到设备 16 kHz。

## 环境变量

配置由 `EnvSchema` 校验，缺必填或格式错误会直接启动失败。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `NODE_ENV` | `development` | `development` / `production` / `test` |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `PORT` | `9494` | HTTP 监听端口 |
| `DATABASE_URL` | 无 | 必填，`mysql://user:pwd@host:3306/db` |
| `DB_ALLOW_PUBLIC_KEY_RETRIEVAL` | `false` | 本地 MySQL `caching_sha2_password` 无 TLS 时通常需设 `true` |
| `JWT_SECRET` | 无 | 必填，至少 32 字符，且需满足基础熵检查 |
| `JWT_EXPIRATION` | `7d` | 秒数或 `15m` / `7d` / `1h` 这类 duration |
| `BLOB_DIR` | `./blobs` | blob 根目录 |
| `QWEATHER_API_KEY` | 空 | 天气动态帧 |
| `QWEATHER_API_HOST` | 空 | QWeather API Host，必须带 `https://` |
| `AI_API_KEY` | 空 | 历史上的今天 AI 优化 |
| `AI_BASE_URL` | 空 | OpenAI-compatible chat completions base URL |
| `AI_MODEL` | `gpt-4o-mini` | AI 优化模型 |
| `TTS_API_KEY` | 空 | TTS provider key |
| `TTS_BASE_URL` | 空 | OpenAI-compatible TTS base URL |
| `TTS_MODEL` | `mimo-v2.5-tts` | TTS 模型 |
| `TTS_DEFAULT_VOICE` | `冰糖` | 默认音色，需属于 shared 的 `TTS_VOICES` |
| `BACKGROUND_WORKERS` | `true` | 是否启动动态刷新后台 worker |

开发环境示例见 [.env.example](.env.example)。

## 本地开发

```bash
cp backend/.env.example backend/.env
bun install
bun run --cwd backend prisma:generate
bun run --cwd backend prisma:migrate
bun run --cwd backend dev
```

`dev` 脚本会执行：

```text
prisma generate && prisma migrate deploy && bun --watch src/main.ts
```

如果需要创建新 migration，使用：

```bash
bun run --cwd backend prisma:migrate
```

## 校验

```bash
bun run --cwd backend lint
bun run --cwd backend typecheck
bun run --cwd backend test
```

根目录也提供聚合命令：

```bash
bun run lint
bun run typecheck
bun run format:check
```

## Docker 运行方式

生产镜像内有两种运行模式，由 `SLATE_RUN_MODE` 控制：

- `server`：默认模式，`entrypoint.sh` 会 `cd /app/backend`，执行 `bunx prisma migrate deploy`，再 `exec bun run src/main.ts`。
- `job`：执行 `bun run scripts/job-runner.ts`，用于可选临时 dashboard 数据推送 sidecar。

frontend `dist/` 位于 `/app/frontend/dist`，由 `main.ts` 自动发现并托管。镜像安装了 ffmpeg，并包含 `backend/scripts/`，因此部署现场的临时 job sidecar 可以复用同一个 Slate 镜像。

Compose 会注入：

```text
DATABASE_URL=mysql://slate:${MYSQL_PASSWORD}@mysql:3306/slate
NODE_ENV=production
PORT=${PORT:-9494}
BLOB_DIR=/data/blobs
```

### Job sidecar

`SLATE_RUN_MODE=job` 时必须设置 `SLATE_JOB`。job runner 会动态加载 `scripts/jobs/<SLATE_JOB>.ts`，该文件需要导出 `job` 或 default `SlateJob`。这是给部署现场临时 sidecar 复用 Slate 镜像的入口。

示例：

```text
SLATE_RUN_MODE=job
SLATE_JOB=sub2api-usage-stats
SLATE_JOB_INTERVAL_SECONDS=600
```

通用变量：

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `SLATE_JOB_INTERVAL_SECONDS` | `600` | 循环运行间隔 |
| `SLATE_JOB_RUN_ONCE` | 空 | 设为 `1` 时只执行一次 |

`scripts/jobs/sub2api-usage-stats.ts` 只支持账号密码登录。它会在进程内缓存 access token，到期后优先使用 refresh token 轮转；refresh 失败或进程重启后才重新用账号密码登录。Sub2API 的 refresh token 按会话独立存储，多个登录会话可以并存。

根 [compose.yml](../compose.yml) 只包含正式 Slate 服务和 MySQL，不固定任何临时 job。需要在某台机器上长期跑某个 dashboard 推送脚本时，在部署目录追加自己的 sidecar 服务和独立 env 文件，不要把外部系统账号密码放进 Slate 主服务 `.env`。

更多脚本结构见 [scripts/README.md](scripts/README.md)。

更多部署步骤见根 [README.md](../README.md)。
