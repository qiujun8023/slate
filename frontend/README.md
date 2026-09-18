# Slate / Frontend

Slate 的 Web 管理端：账号、设备绑定、内容组、图片与动态内容编辑、帧预览。

React 19 + React Router 7 + Vite 8 + Tailwind v4。数据请求用 TanStack Query 5 + axios，组件原语用 Radix UI，拖拽用 dnd-kit，图标用 lucide-react。

## 目录

```text
src/
├── app/          入口、Provider、路由（routes.ts 定义路径，App.tsx 定义路由表）
├── pages/        路由页面
├── features/     按业务域划分：auth / devices / groups / contents / dynamic
├── components/   跨业务的 UI：ui 基础组件、layout、feedback（Toast / Confirm）、dnd、eink 帧预览
├── hooks/        跨业务的 hooks
├── lib/          axios 实例、错误处理、格式化、帧解码
└── styles/global.css  设计 token
```

- 只属于某个业务域的组件、hook、查询都放在 `features/<域>/` 下；只有跨域复用的才放到 `components/`、`hooks/`、`lib/`。
- 查询 hook 放在 `features/<域>/query/`，页面和组件从这里导入。

## 路由

| path | 页面 |
| --- | --- |
| `/login`、`/register` | `AuthPage` |
| `/` | `DashboardPage`：设备与内容组总览 |
| `/devices/:did` | `DashboardPage`，直接打开设备弹窗 |
| `/groups/:gid` | `GroupDetailPage`：组内内容列表与排序 |
| `/groups/:gid/contents/new` | `ContentNewPage`：新建图片或动态内容 |
| `/groups/:gid/contents/image/:contentId/edit` | `ImageContentEditorPage` |
| `/groups/:gid/contents/dynamic/:contentId/edit` | `DynamicContentEditorPage` |

除登录和注册外，页面都需要登录。

## 实现要点

- **鉴权**：JWT 存在 localStorage，由 axios 拦截器附加到请求上；收到 401 时清除登录态并跳转到登录页。
- **图片编辑**：在浏览器里裁剪到 400×300，用 shared 的处理流程生成预览；保存时上传预览 canvas 导出的 PNG，后端再用同一流程生成最终的 1bpp 帧。
- **动态内容**：配置表单覆盖 shared 中的所有类型，预览调用 `POST /api/v1/contents/preview`，返回的 1bpp 数据画到 canvas 上。
- **Dashboard**：编辑页展示推送 URL（`POST /api/v1/contents/:id/data`），这个 URL 本身就是凭证。
- **缓存**：
  - 图片和音频的 query key 带 etag，永不过期；
  - 设备列表每 30 秒刷新一次；
  - 有音频正在生成时，每 2.5 秒轮询一次；
  - 拖拽排序先乐观更新，失败时回滚。
- **开发代理**：`/api` 和 `/healthz` 转发到 `localhost:9494`。生产环境由后端同域托管 `dist/`。

## 设计系统 Mono Press

报刊编辑风格，token 定义在 [src/styles/global.css](src/styles/global.css) 的 `@theme` 中。颜色、圆角、字体都用 token，不写裸色值。

- 配色：纸本底 `paper` #f5f3ed、墨色 `ink` #14110d、次要文字 `stone`、分隔线 `line`。砖红 `clay` #a8281c 只用于危险操作、错误和低电量。
- 字体：标题用 `serif`（Noto Serif SC），界面用 `sans`（IBM Plex Sans），数字和代码用 `mono`（IBM Plex Mono）。
- 形态：所有圆角都是 0px（全局覆写，Radix 浮层除外）；1px 墨线；不用模糊。
- 页面 header：标题在上，meta 在下，不用眉题。

## 检查

```bash
bun run --cwd frontend lint
bun run --cwd frontend typecheck
bun run --cwd frontend build
```
