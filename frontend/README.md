# Slate / Frontend

前端是 Slate 的 Web 管理端，用于登录注册、绑定设备、管理内容组、创建/编辑内容、预览动态帧，以及生成 dashboard 数据推送 URL。

## 技术栈

| 层 | 选型 |
| --- | --- |
| 框架 | React 19 + React Router v7 |
| 构建 | Vite 8 |
| 样式 | Tailwind v4，token 在 `src/styles/global.css` |
| 组件原语 | Radix UI Dialog / DropdownMenu / Select / Toast |
| 数据 | TanStack Query 5 + axios |
| 拖拽排序 | dnd-kit |
| 图标 | lucide-react |
| 上传 | react-dropzone |
| 共享类型 | workspace 包 `shared` |

## 目录

```text
frontend/src/
├── app/
│   ├── main.tsx              BrowserRouter、QueryClientProvider、Auth/Toast/Confirm Provider
│   └── App.tsx               路由表与 lazy page
├── components/
│   ├── dnd/                  SortableGrid 与拖拽排序 helper
│   ├── eink/                 1bpp 帧预览与状态栏 overlay
│   ├── feedback/             ErrorBoundary、Toast、Confirm
│   ├── layout/               Layout、PageHeader、Section、RequireRouteParams
│   └── ui/                   Button、Input、Select、Dialog 等基础组件
├── features/
│   ├── auth/                 AuthProvider、AuthLayout、RequireAuth、登录注册 hooks、重定向逻辑
│   ├── contents/             内容列表、卡片、新建页、图片编辑、音频预览、content queries
│   ├── devices/              设备卡片、绑定弹窗、设备详情弹窗、device queries
│   ├── dynamic/              动态内容配置表单、预览、默认 config、dashboard push panel
│   └── groups/               内容组卡片、新建弹窗、group queries
├── hooks/                    跨 feature hooks
├── lib/                      axios、错误解包、格式化、JSON helper、样式片段、eink 图片解码
├── pages/
│   ├── auth/                 LoginPage、RegisterPage
│   ├── contents/             ContentNewPage、ImageContentEditorPage、DynamicContentEditorPage
│   ├── dashboard/            DashboardPage
│   └── groups/               GroupDetailPage
└── styles/global.css          Tailwind v4 + Mono Press design tokens
```

## 目录约定

- `features/*` 放业务域内聚的组件、hooks、queries、lib。
- `components/ui` 只放可复用 UI 组件；纯 hook 不放在这里。
- `components/feedback` 放全局反馈能力（Toast、Confirm、mutation feedback helper）。
- `hooks` 只放跨 feature hooks；和某个领域强绑定的 hook 放回对应 `features/*`。
- `lib` 只放跨业务域的基础设施和通用工具。

## 路由

路径常量和 path builder 定义在 [src/app/routes.ts](src/app/routes.ts)，路由表在 [src/app/App.tsx](src/app/App.tsx)。

| path | 页面 | 说明 |
| --- | --- | --- |
| `/login` | `LoginPage` | 邮箱/用户名 + 密码登录 |
| `/register` | `RegisterPage` | 注册并保存 JWT |
| `/` | `DashboardPage` | 设备与内容组总览 |
| `/devices/:did` | `DashboardPage` | deep link 到设备弹窗 |
| `/groups/:gid` | `GroupDetailPage` | 单个内容组内容列表、排序、编辑入口 |
| `/groups/:gid/contents/new` | `ContentNewPage` | 图片和动态内容统一新建入口 |
| `/groups/:gid/contents/image/:contentId/edit` | `ImageContentEditorPage` | 编辑图片内容 |
| `/groups/:gid/contents/dynamic/:contentId/edit` | `DynamicContentEditorPage` | 编辑动态内容 |
| `*` | redirect `/` | 兜底 |

除登录/注册外，页面都包在 `RequireAuth + Layout` 下。

## 主要工作流

### 设备

- Dashboard 显示当前账号的设备列表。
- `AddDeviceDialog` 输入设备屏幕上的 6 位 pair code，调用 `POST /api/v1/devices/claims`。
- `DeviceModal` 支持重命名、选择当前内容组、解绑。
- 设备列表每 30 秒 refetch，用于刷新 `last_seen_at`、电量、RSSI、固件版本。

### 内容组

- Dashboard 显示内容组列表。
- 支持创建、重命名、删除、拖拽排序。
- 进入 `/groups/:gid` 后管理该内容组下的内容，内容也支持拖拽排序。

### 图片内容

图片编辑器支持：

- 上传图片并裁剪/平移/缩放到 400 x 300。
- 调整阈值与 dither 模式。
- 浏览器端使用 `shared` 的 `rgbaToGray -> autoInvert -> autoContrast -> ditherToBinary` 生成预览。
- 可附加上传音频，或提交 TTS 文案由后端生成音频。
- 保存时提交 `multipart/form-data`，图片字段来自预览 canvas 导出的 PNG。

后端仍会重新用 sharp + shared 管线生成最终 1bpp `.img`，前端预览是为了让用户尽量所见即所得。

### 动态内容

统一动态配置表单支持当前 shared 中的所有类型：

- 日历：`daily_calendar`
- 月历：`month_calendar`
- 天气：`weather`
- 历史上的今天：`history_today`
- 气象预警：`weather_alert`
- 地震速报：`earthquake_report`
- 外部数据 dashboard：`dashboard`
- 字体测试：`font_test`
- 热榜：`hot_list`

动态预览调用：

```text
POST /api/v1/contents/preview
POST /api/v1/contents/:contentId/preview
```

响应是 400 x 300 1bpp binary，前端通过 `DynamicFramePreview` 转成 canvas 预览。

### Dashboard 推送

dashboard 动态内容可以选择系统模板或自定义 JSON 模板：

- 系统模板：`ai_usage_stats`、`ai_quota_monitor`
- 自定义模板：编辑 `DashboardTemplate` JSON

创建时必须提供初始数据。编辑已有 dashboard 内容时，`DashboardPushPanel` 会展示：

```text
POST /api/v1/contents/:contentId/data
```

这个 URL 使用 `contentId` 作为 capability 凭证，不需要 JWT。前端只负责展示和复制 URL；泄漏后需要删除内容重建。

## API 客户端

[src/lib/http.ts](src/lib/http.ts) 中的 axios 实例：

- `baseURL: '/'`
- `timeout: 30000`
- request interceptor 从 `localStorage` 读取 JWT 并设置 `Authorization: Bearer <token>`
- response interceptor 遇到 401 调用 `notifyUnauthorized()`，清理本地登录态并跳转登录

dev 模式由 Vite proxy 转发：

```ts
server: {
  port: 5173,
  proxy: {
    '/api': { target: 'http://localhost:9494', changeOrigin: true },
    '/healthz': { target: 'http://localhost:9494', changeOrigin: true },
  },
}
```

生产模式中 `dist/` 由 backend 同域托管，不需要单独部署前端。

## 数据与缓存

全局 QueryClient 默认配置在 [src/app/main.tsx](src/app/main.tsx)：

```ts
queries: {
  retry: 1,
  refetchOnWindowFocus: false,
  staleTime: 30_000,
}
```

约定：

- query key 和接口 hooks 按 feature 拆分在 `features/*/query/`。
- 页面和组件从对应 feature 的 `query/*-queries.ts` 导入具体 hooks。
- mutation 成功后 invalidate 相关 group/device/content query。
- 图片和音频 binary query key 带 etag，`staleTime: Infinity`，etag 不变就不重拉。
- 内容列表如果存在 `pending` / `generating` 音频，会每 2.5 秒轮询直到完成。
- 拖拽排序使用乐观更新，失败时回滚本地顺序。

## 类型来源

前端通过 Vite alias 直接消费 `shared/src`：

```ts
resolve: {
  alias: {
    '@': path.resolve(__dirname, 'src'),
    shared: path.resolve(__dirname, '../shared/src'),
  },
}
```

常见导入：

```ts
import type { ContentDetailT, DeviceSummaryT, DynamicConfigT } from 'shared';
import { DynamicConfig, DEFAULT_DITHER_MODE, ditherToBinary } from 'shared';
```

`shared` 不需要先 build；workspace 依赖和 alias 都指向源码。

## 设计系统 Mono Press

设计 token 位于 [src/styles/global.css](src/styles/global.css)。整体是报刊编辑风格：

- 纸本底色：`--color-paper: #f5f3ed`
- 墨黑主色：`--color-ink: #14110d`
- 砖红警示：`--color-clay: #a8281c`
- 边线：`--color-line: #d8d2bf`
- 0px 圆角，硬边框，少量硬阴影
- 标题使用 serif 栈，UI 使用 sans，数字/代码使用 mono

字体 token：

| token | 用途 |
| --- | --- |
| `--font-serif` | 标题、中文锚点 |
| `--font-sans` | 默认 UI |
| `--font-mono` | 数字、代码、状态标签 |

`global.css` 会全局覆写圆角为 0，但排除了 Radix popper/menu/select 等浮层元素，避免原语布局异常。

## Vite chunk 策略

[vite.config.ts](vite.config.ts) 的 `manualChunks` 按依赖稳定性拆包：

| chunk | 包 |
| --- | --- |
| `react-vendor` | `react`、`react-dom`、`react-router-dom`、`scheduler` |
| `radix-vendor` | `@radix-ui/*` |
| `dnd-vendor` | `@dnd-kit/*` |
| `lucide-vendor` | `lucide-react` |

业务代码变更时尽量不影响 vendor chunk 缓存。

## 本地开发

```bash
bun install
bun run dev:frontend
```

访问：

```text
http://localhost:5173
```

需要同时运行后端：

```bash
bun run dev:backend
```

## 校验

```bash
bun run --cwd frontend lint
bun run --cwd frontend typecheck
bun run --cwd frontend build
```

根目录聚合命令也会覆盖前端：

```bash
bun run lint
bun run typecheck
bun run format:check
```
