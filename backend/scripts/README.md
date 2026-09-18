# Backend Scripts

```text
scripts/
├── job-runner.ts     job 入口，Docker 中 SLATE_RUN_MODE=job 时运行
├── jobs/             dashboard 数据推送任务
├── lib/              job 共用的环境变量、HTTP、推送 helper
├── helpers/          维护脚本共用的 Nest 启动与日志
├── maintenance/      一次性创建内容组
├── fonts/            位图字体提取与生成
└── debug/            本地渲染调试
```

## Job

`SLATE_JOB=<name>` 会加载 `jobs/<name>.ts` 导出的 `job`。新增 job 只需新建文件，无需注册。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `SLATE_JOB` | — | 必填，job 文件名 |
| `SLATE_API_BASE` | — | 必填，Slate 地址，如 `http://slate:9494` |
| `SLATE_JOB_INTERVAL_SECONDS` | `600` | 循环间隔 |
| `SLATE_JOB_RUN_ONCE` | — | 设为 `1` 时只运行一次 |
| `SLATE_JOB_TIME_ZONE` | `Asia/Shanghai` | 时间显示用的时区 |

本地运行一次：

```bash
cd backend
SLATE_JOB=sub2api-usage-stats SLATE_JOB_RUN_ONCE=1 bun run scripts/job-runner.ts
```

部署时在自己的 compose 里加一个 sidecar，复用 Slate 镜像。外部系统的账号只写在这个 service 里，不要放进 Slate 主服务的 `.env`，也不要加到仓库的 `compose.yml`：

```yaml
services:
  slate-sub2api-stats:
    image: ghcr.io/qiujun8023/slate:latest
    restart: unless-stopped
    environment:
      SLATE_RUN_MODE: job
      SLATE_JOB: sub2api-usage-stats
      SLATE_API_BASE: http://slate:9494
      SUB2API_BASE: https://sub2api.example.com
      SUB2API_CONTENT_ID: <dashboard 内容 ID>
      SUB2API_EMAIL: you@example.com
      SUB2API_PASSWORD: change_me
```

### sub2api-usage-stats

把 Sub2API 用量推送到 `ai_usage_stats` 模板的 dashboard。需要 `SUB2API_BASE`、`SUB2API_CONTENT_ID`、`SUB2API_EMAIL`、`SUB2API_PASSWORD`。

用账号密码登录，access token 只缓存在进程内，过期后用 refresh token 续期，续期失败或进程重启才重新登录。不支持 2FA / Turnstile。

### claude-code-quota-monitor

把 Claude Code 限额推送到 `ai_quota_monitor` 模板的 dashboard。需要 `CLAUDE_QUOTA_CONTENT_ID`；可选 `ANTHROPIC_API_KEY`（默认读取 `~/.claude/.credentials.json`）、`ANTHROPIC_API_BASE`、`CLAUDE_PLAN_LABEL`。

也可以直接配成 Claude Code 的 statusLine 命令：从 stdin 读取限额，输出状态栏并在后台推送，推送频率由 `CLAUDE_QUOTA_PUSH_INTERVAL_MS` 控制（默认 60000）。

## 其他脚本

在 `backend/` 下直接运行，例如：

```bash
bun run scripts/maintenance/create-hot-list-group.ts
bun run scripts/debug/render-dynamic-debug.ts
bash scripts/fonts/generate-font-test-assets.sh
```
