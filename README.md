# Slate

Slate（墨笺）是一个面向 400 × 300 黑白墨水屏的开源相框、信息看板和语音玩具。

照片、实时资讯和自定义仪表板推送到墨水屏，按键翻页，翻到时自动朗读。仓库包含设备固件、Go 后端和 Web 管理端，可以完全自托管。

![Slate 软件管理、设备同步和墨水屏内容形态总览](readme-hero.png)

## 功能

内容按「内容组」组织。一组里可以混放静态图片和动态内容，设备上按键翻页轮播。

### 静态图片

在浏览器里裁剪、缩放并抖动成 1bpp 帧，所见即上屏。可选 6 种抖动算法：

| 算法 | 适合 |
| --- | --- |
| `floyd` | 照片（推荐） |
| `atkinson` | 高对比 |
| `sierra` | 柔和 |
| `bayer4` / `bayer8` | 粗 / 细网点 |
| `threshold` | 线稿 |

### 动态内容

后端拉取数据渲染成帧，到点自动刷新。

| 类型 | 说明 | 可配置项 |
| --- | --- | --- |
| 日历 | 农历、公历、节日，分日历和月历 | — |
| 天气 | 和风天气实况与预报 | 城市 |
| 历史上的今天 | 每日历史事件 | 数据源：Wikipedia 或百度百科 |
| 气象预警 | 官方气象预警 | 省份或全国 |
| 地震速报 | 全国最新地震 | — |
| 热榜 | 86 个榜单源，分 5 类 | 榜单源 |
| 信息仪表板 | 外部程序推送数据，按模板渲染 | 模板或自定义布局 |
| 字体测试 | 26 种点阵字体上屏预览 | 字体、反色 |

除字体测试外都可以设置刷新间隔；日历、天气、历史上的今天还可以设置时区。

### 音频

图片和大部分动态内容可以挂一段音频，翻到该帧时播放。

- **上传**：wav、mp3、aac、m4a、ogg、opus、flac、wma，超过 60 秒的部分截掉。
- **TTS**：给图片填写朗读文案，用 OpenAI 兼容接口合成。
- **自动朗读**：日历、天气、历史上的今天、气象预警、地震速报的朗读文案随数据自动生成。

### 信息仪表板

用 JSON 描述区块布局：文本、指标、进度条、趋势线、矩形、直线。

外部程序向 `POST /api/contents/{id}/data` 推送数据即可上屏，适合 CI 状态、家庭传感器、AI 用量等。内置「AI 限额监控」模板，可用于 Claude Code、Codex 等服务。

### 语音对话

固件集成小智（xiaozhi）协议，双击确认键进入对话。

### 设备

- **配网**：首次开机开启热点和配网页，填写 Wi-Fi 与服务端地址。
- **绑定**：屏幕显示 6 位配对码，在 Web 端输入即可绑定。
- **同步**：图片和音频按 sha256 增量同步，缓存在本地。
- **省电**：闲置自动深睡；动态帧按服务端给的刷新时间定时唤醒。

## 仓库结构

| 目录 | 内容 |
| --- | --- |
| [`backend/`](backend/README.md) | Go 服务：Web API、设备协议、动态内容渲染、音频转码与 TTS，同时托管前端 |
| [`frontend/`](frontend/README.md) | React 19 + Vite + Tailwind v4 的 Web 管理端 |
| [`firmware/`](firmware/README.md) | ESP-IDF 5.5 固件，目标板 ZecTrix Note4（ESP32-S3 + 4.2" EPD） |
| `scripts/` | 提交前检查 `check.sh`（CI 也跑它）和字体生成脚本 |

根目录的 `Dockerfile` 构建包含前后端的单一镜像，`compose.yml` 是自托管样例。

---

## 部署

准备 compose 文件和密钥，然后启动：

```bash
curl -fLO https://github.com/qiujun8023/slate/releases/latest/download/compose.yml
cat > .env <<EOF
POSTGRES_PASSWORD=$(openssl rand -hex 32)
JWT_SECRET=$(openssl rand -hex 64)
EOF
mkdir -p slate postgres
sudo chown -R 1000:1000 slate   # 容器以 uid 1000 运行
docker compose up -d
```

打开 `http://<主机>:9494/register` 注册第一个账号。

天气、AI、TTS 等可选配置加到 `.env`，变量见 [backend/README.md](backend/README.md#配置)。

**升级**：`docker compose pull && docker compose up -d`，启动时自动迁移数据库。

**镜像 tag**：

| tag | 含义 |
| --- | --- |
| `latest`、`vX.Y.Z`、`vX.Y` | 稳定版 |
| `master` | 主干最新构建 |
| 7 位 commit SHA | 固定到某次提交 |

### 从 v0.1.x 升级

v0.1.x 用 MySQL，当前版本用 PostgreSQL，数据用一次性的 `slate import` 搬过去。

账号、设备（含配对关系，不用重新绑定）、内容组、内容、图片和音频都会迁移。设备需要刷入同版本固件，旧固件连不上新后端。

在原来的部署目录里执行：

```bash
docker compose down
cp -a slate slate.bak && cp -a mysql mysql.bak            # 先备份
mv compose.yml compose.v0.1.yml
curl -fLO https://github.com/qiujun8023/slate/releases/latest/download/compose.yml
echo "POSTGRES_PASSWORD=$(openssl rand -hex 32)" >> .env  # .env 里原有的 JWT_SECRET 继续用
mkdir -p postgres

docker compose -f compose.v0.1.yml up -d mysql            # 只启动旧 MySQL 作为数据源
docker compose up -d postgres
docker compose run --rm slate import "mysql://slate:<MYSQL_PASSWORD>@mysql:3306/slate"
docker compose -f compose.v0.1.yml rm -sf mysql
docker compose up -d
```

导入要求目标库里还没有账号。所有数据在一个事务里写入，失败不会留下半份数据。

确认无误后，可以删掉 `compose.v0.1.yml`、`./mysql`、备份，以及 `./slate/blobs` 下除 `sha256/` 之外的旧文件。

---

## 本地开发

需要：

- Go，版本见 `backend/go.mod`
- Bun，版本见 `frontend/package.json`
- PostgreSQL 18 和 ffmpeg
- 构建固件另需 ESP-IDF 5.5

```bash
docker run -d --name slate-postgres -p 5432:5432 \
  -e POSTGRES_DB=slate -e POSTGRES_USER=slate -e POSTGRES_PASSWORD=slate \
  postgres:18-alpine

cp backend/.env.example backend/.env
(cd backend && go run ./cmd/slate)           # http://localhost:9494
(cd frontend && bun install && bun run dev)  # http://localhost:5173，/api 代理到 9494
```

打开 `http://localhost:5173/register` 注册账号。提交前运行 `scripts/check.sh`。

固件的构建和烧录见 [firmware/README.md](firmware/README.md)。

## 贡献与发布

开发约定、提交前检查和发版流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。
