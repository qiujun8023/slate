# Backend Scripts

`backend/scripts/` 只放随 Slate 代码一起版本管理的辅助程序。可选临时 dashboard 推送任务走统一 job runner；一次性维护、调试、字体生成脚本保留真实路径，不提供旧入口兼容。

## 目录

```text
scripts/
├── job-runner.ts                  Docker sidecar / cron-like job 入口
├── jobs/                          可选临时 dashboard 推送任务
│   ├── sub2api-usage-stats.ts     Sub2API 用量统计 -> ai_usage_stats
│   └── claude-code-quota-monitor.ts Claude Code 限额 -> ai_quota_monitor
├── lib/                           job 共享 env / HTTP / Slate ingest helper
├── helpers/                       维护脚本共享 Nest bootstrap 和日志 helper
├── maintenance/                   一次性创建或修正内容组
├── fonts/                         位图字体提取和生成工具
└── debug/                         本地渲染调试
```

## Job Runner

本地单次运行：

```bash
cd backend
SLATE_JOB=sub2api-usage-stats SLATE_JOB_RUN_ONCE=1 bun run scripts/job-runner.ts
```

长期循环运行：

```bash
cd backend
SLATE_JOB=sub2api-usage-stats SLATE_JOB_INTERVAL_SECONDS=600 bun run scripts/job-runner.ts
```

生产 Docker 通过 `SLATE_RUN_MODE=job` 进入 job runner。`SLATE_JOB=<name>` 会动态加载 `scripts/jobs/<name>.ts`，该文件导出 `job` 或 default `SlateJob`：

```yaml
environment:
  - SLATE_RUN_MODE=job
  - SLATE_JOB=sub2api-usage-stats
  - SLATE_JOB_INTERVAL_SECONDS=600
  - SLATE_JOB_TIME_ZONE=Asia/Shanghai
  - SLATE_API_BASE=http://slate:9494
```

`SLATE_JOB_INTERVAL_SECONDS` 默认 600。`SLATE_JOB_RUN_ONCE=1` 只执行一次后退出，适合临时验证。新增临时 job 只需要新增 `scripts/jobs/<name>.ts`，不需要改中心注册表。

## Sub2API Usage Stats

`sub2api-usage-stats` 使用 Sub2API 的用户登录接口：

- `POST /api/v1/auth/login`，body 为 `email` 和 `password`。
- 登录返回 `access_token`、`refresh_token`、`expires_in`。
- access token 只缓存在当前进程内；到期前复用，不落盘。
- access token 过期后优先用 `POST /api/v1/auth/refresh` 轮转 refresh token；refresh 失败才重新用账号密码登录。
- 2FA / Turnstile 登录不属于这个自动化任务的支持范围。

所需环境变量：

```text
SLATE_RUN_MODE=job
SLATE_JOB=sub2api-usage-stats
SLATE_JOB_INTERVAL_SECONDS=600
SLATE_JOB_TIME_ZONE=Asia/Shanghai
SLATE_API_BASE=http://slate:9494
SUB2API_BASE=https://sub2api.example.com
SUB2API_CONTENT_ID=slate_dashboard_content_id
SUB2API_EMAIL=you@example.com
SUB2API_PASSWORD=change_me
```

Sub2API 的 refresh token 是按会话单独存储和撤销的，多端登录可以并存。这个 job 不会每轮重新登录，正常情况下只在首次启动、refresh 失败或进程重启后用账号密码登录。

## Compose 配置边界

临时 job sidecar 的配置直接写在部署现场 compose 的该 job service `environment` 中；不要把外部系统账号密码混入主服务环境，也不要把特定临时 job 放进 release 用根 `compose.yml`。

示例：

```yaml
services:
  slate-sub2api-usage-stats:
    image: ghcr.io/qiujun8023/slate:latest
    restart: unless-stopped
    environment:
      SLATE_RUN_MODE: job
      SLATE_JOB: sub2api-usage-stats
      SLATE_JOB_INTERVAL_SECONDS: '600'
      SLATE_JOB_TIME_ZONE: Asia/Shanghai
      SLATE_API_BASE: http://slate:9494
      SUB2API_BASE: https://sub2api.example.com
      SUB2API_CONTENT_ID: slate_dashboard_content_id
      SUB2API_EMAIL: you@example.com
      SUB2API_PASSWORD: change_me
```

这些具体 job 的 compose 片段属于部署现场配置，不放进 release 文档或根 `compose.yml`。

## Maintenance / Debug / Fonts

这些脚本需要显式路径运行：

```bash
bun run scripts/maintenance/create-hot-list-group.ts
bun run scripts/maintenance/create-font-test-group.ts
bun run scripts/maintenance/create-vehicle-group.ts
bun run scripts/debug/render-dynamic-debug.ts
bash scripts/fonts/generate-font-test-assets.sh
```

它们不是 Docker entrypoint 的运行模式。需要以 sidecar 运行时才新增到 `jobs/`，并让部署现场通过 `SLATE_JOB=<name>` 选择。
