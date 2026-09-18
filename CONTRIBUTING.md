# 贡献指南

欢迎 issue 与 PR。环境搭建见 [README.md](README.md)，各模块细节见对应目录的 README。

## 报告 Bug

请写清：

- 复现步骤、期望行为、实际行为
- 版本：固件 `fw_version`（设置 → 设备信息）、后端镜像 tag 或 commit、浏览器版本
- 相关日志：`docker compose logs slate`、固件串口日志（`idf.py monitor`）、浏览器 Console

## 提交 PR

- 从 `master` 切分支，一个 PR 只做一件事。
- 提交信息格式为 `<type>(<scope>): <中文摘要>`，需要时在 body 里用 `- ` 列出改动。`type` 常用 `feat` / `fix` / `refactor` / `perf` / `docs` / `chore`，`scope` 用模块名，如 `backend` / `frontend` / `firmware` / `dynamic` / `app`（跨端）。

```text
fix(backend): 修复图片内容创建参数校验

- 避免 multipart 分支被全局 Zod pipe 重复校验
- 增加创建与编辑接口的回归测试
```

提交前运行（与 CI 一致）：

```bash
bun run format:check     # 格式问题用 bun run format 修复
bun run lint
bun run typecheck
bun run --cwd backend test
bun run --cwd frontend build
```

改了固件需要 ESP-IDF 5.5 构建通过（CI 用 v5.5.2）：

```bash
source $IDF_PATH/export.sh
idf.py -C firmware build
```

涉及 EPD、电源、按键、休眠的改动，请在 PR 里说明实机验证了哪些场景；改动分区表或 NVS 字段时，说明对已部署设备 OTA 升级的影响。

前端 UI 改动遵守 [Mono Press 设计系统](frontend/README.md#设计系统-mono-press)。

## 发布版本

一个 `vX.Y.Z` tag 同时发布 Docker 镜像、固件和 GitHub Release，后端与固件不分开发版。

1. 把以下版本号改成 `X.Y.Z`，然后运行 `bun install` 同步 `bun.lock`：
   - `package.json`、`backend/package.json`、`frontend/package.json`、`shared/package.json`
   - `firmware/sdkconfig.defaults` 的 `CONFIG_APP_PROJECT_VER`
2. 提交后打 annotated tag，tag message 即 Release notes：

   ```bash
   git tag -a v0.2.0
   git push origin v0.2.0
   ```

   ```text
   Slate v0.2.0

   - 后端 / Web：新增 ...
   - 固件：修复 ...
   ```

推送 tag 后 `release.yml` 会校验版本号与 tag body、跑完整检查、推送镜像（`vX.Y.Z` / `X.Y` / `latest` / `sha-*`）、构建固件，并把固件、sha256、`compose.yml`、`slate.env.example` 上传到 Release。只有最新的 tag 能发布，防止 `latest` 回退。

`master` 上的 `docker.yml` / `firmware.yml` 是滚动构建，不代表稳定版本。
