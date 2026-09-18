# Slate

Slate（墨笺）是一个面向 400 × 300 黑白墨水屏的开源相框 / 信息看板 / 语音玩具。照片、实时资讯和自定义仪表板推送到墨水屏上，按键翻页，翻到时自动朗读。仓库包含设备固件、后端、Web 管理端和共享 schema，可以完全自托管。

![Slate 软件管理、设备同步和墨水屏内容形态总览](readme-hero.png)

## 功能

内容按「内容组」组织，一组内可以混放静态图片和动态内容，设备按键翻页轮播。

**静态图片**：浏览器内裁剪、缩放并预览抖动效果，后端用 sharp 渲染成 1bpp。支持 6 种抖动算法：`threshold`（线稿）、`bayer4` / `bayer8`（粗 / 细网点）、`floyd`（照片，推荐）、`atkinson`（高对比）、`sierra`（柔和）。

**动态内容**：后端拉取数据渲染成帧，到点自动刷新。

| 类型 | 说明 | 可配置项 |
| --- | --- | --- |
| 日历（日 / 月） | 农历、公历、节日 | — |
| 天气 | 和风天气实况与预报 | 城市 |
| 历史上的今天 | 每日历史事件 | 数据源：Wikipedia / 百度百科 |
| 气象预警 | 官方气象预警 | 31 个省级行政区或全国 |
| 地震速报 | 全国最新地震 | 刷新间隔 |
| 热榜 | 86 个榜单源，分综合 / 新闻 / 科技 / 社区 / 消费 5 类 | 榜单源、刷新间隔 |
| 信息仪表板 | 外部 API 推送数据，按模板渲染 | 内置模板（AI 使用统计 / AI 限额监控）或自定义布局 |
| 字体测试 | 26 种点阵字体上屏预览 | 字体、反色 |

**音频**：静态图片和除仪表板、热榜、字体测试外的动态内容可以挂一段音频，翻到该帧时播放。音频可以上传（ffmpeg 转 16 kHz 单声道 PCM），也可以填文案用 OpenAI 兼容 TTS 合成。

**语音对话**：固件集成小智（xiaozhi）协议。

**信息仪表板**：用 JSON 描述区块布局（文本、指标、进度条、趋势线、矩形、直线），外部程序向 `POST /api/v1/contents/:id/data` 推送数据即可上屏，适合 CI 状态、家庭传感器、AI 用量等。

**设备**：
- 首次开机开启热点和配网页，填 Wi-Fi 与服务端地址；屏幕显示 6 位配对码，在 Web 端输入即可绑定。
- 按 ETag 增量同步图片和音频，本地 LittleFS 缓存。
- 闲置自动深睡；动态帧按下次刷新时间定时唤醒，只刷新当前帧。

## 仓库结构

| 目录 | 内容 | 文档 |
| --- | --- | --- |
| `backend/` | Bun + NestJS 11 + Fastify + Prisma 7 + MySQL 8；API、设备协议、帧渲染、音频 | [backend/README.md](backend/README.md) |
| `frontend/` | React 19 + Vite 8 + Tailwind v4 Web 管理端 | [frontend/README.md](frontend/README.md) |
| `shared/` | 前后端共享的 zod schema、动态内容配置、抖动与图像预处理 | [shared/README.md](shared/README.md) |
| `firmware/` | ESP-IDF 5.5 固件，目标板 ZecTrix Note4（ESP32-S3 + 4.2" EPD） | [firmware/README.md](firmware/README.md) |

根目录的 `compose.yml` 是自托管示例，`Dockerfile` 构建包含前后端的单一镜像。

## 部署

从最新 Release 下载部署文件：

```bash
curl -fLO https://github.com/qiujun8023/slate/releases/latest/download/compose.yml
curl -fLo .env https://github.com/qiujun8023/slate/releases/latest/download/slate.env.example
```

在 `.env` 里填写 `MYSQL_PASSWORD`（`openssl rand -hex 32`）和 `JWT_SECRET`（`openssl rand -hex 64`），按需填写天气、AI、TTS 配置。然后：

```bash
mkdir -p slate mysql
sudo chown -R 1000:1000 slate   # 容器以 uid 1000 运行
docker compose up -d
```

打开 `http://<host>:9494/register` 注册第一个账号。数据在 `./slate`（图片、音频）和 `./mysql`。升级：`docker compose pull && docker compose up -d`。

镜像 tag：`latest` / `vX.Y.Z` / `X.Y` 为稳定版，`master` 为主干最新构建，`sha-<short>` 固定到某次提交。

## 本地开发

需要 Bun 1.x、MySQL 8、ffmpeg；构建固件另需 ESP-IDF 5.5。

```bash
docker run -d --name slate-mysql -p 3306:3306 \
  -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=slate \
  -e MYSQL_USER=slate -e MYSQL_PASSWORD=slate mysql:8.4

bun install
cp backend/.env.example backend/.env
bun run dev:backend     # http://localhost:9494，启动时自动执行迁移
bun run dev:frontend    # http://localhost:5173，/api 代理到 9494
```

打开 `http://localhost:5173/register` 注册账号。本地后端只读 `backend/.env`，根目录的 `.env` 仅供 Docker Compose 使用。

固件：

```bash
source $IDF_PATH/export.sh
idf.py -C firmware build
idf.py -C firmware -p <serial> flash monitor
```

## 贡献与发布

开发约定、提交前检查和发版流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。
