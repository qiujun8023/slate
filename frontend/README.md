# Slate / Frontend

Slate 的 Web 管理端：账号、设备绑定、内容组、图片与动态内容编辑、帧预览。

React 19 + React Router 7 + Vite 8 + Tailwind v4，TypeScript。数据请求用 TanStack Query 5 + axios，组件原语用 Radix UI，拖拽用 dnd-kit，图标用 lucide-react。

## 目录

```text
src/
├── api/          与后端的契约：generated.ts 和 catalog.json 由后端 go generate 生成，不要手改
├── app/          入口、Provider、路由（routes.ts 定义路径，App.tsx 定义路由表）
├── pages/        路由页面
├── features/     按业务域划分：auth / devices / groups / contents / dynamic
├── components/   跨业务的 UI：ui 基础组件、layout、feedback（Toast / Confirm）、dnd、eink 帧预览
├── hooks/        跨业务的 hooks
├── lib/          axios 实例、错误处理、格式化；eink/ 是帧的抖动、打包与解码
└── styles/global.css  设计 token
```

- 只属于某个业务域的组件、hook、查询放在 `features/<域>/`；跨域复用的才放到 `components/`、`hooks/`、`lib/`。
- 查询 hook 放在 `features/<域>/query/`，页面和组件从这里导入。
- 请求和响应类型一律从 `@/api` 导入。后端改了 `internal/api` 就重新生成，不在前端重复声明。

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

除登录和注册外都需要登录。

## 实现要点

- **请求**：axios 的 `baseURL` 是 `/api`，`lib/http.ts` 统一取出 envelope 的 `data`，错误提示由 `lib/api-errors.ts` 从 envelope 的 `message` 读取。JWT 存在 localStorage，收到 401 时清除登录态并跳到登录页。
- **图片**：浏览器完成裁剪、缩放、灰度、自动反色、自动对比度和抖动，`renderFrame` 生成 400×300 packed 1bpp（15000 字节）直接上传，服务端原样保存。预览和上传走同一套函数，看到的就是设备上的样子。抖动模式列表在 `lib/eink/dither.ts`，`lib/eink/testdata/dither.json` 固定了各模式的输出，改算法要有意更新它。
- **动态内容**：配置表单覆盖所有类型，选项（字体、模板、榜单源、音色）来自 `catalog.json`。预览调用 `POST /api/contents/preview`，返回的 1bpp 画到 canvas 上。
- **Dashboard**：编辑页展示推送 URL（`POST /api/contents/{id}/data`），这个 URL 本身就是凭证。
- **缓存**：帧和音频按 blob hash 做 query key，永不过期；设备列表每 30 秒刷新；有音频在生成时每 2.5 秒轮询；拖拽排序先乐观更新，失败回滚。
- **开发代理**：`bun run dev` 把 `/api` 转发到 `localhost:9494`。生产环境由后端同域托管 `dist/`。

## 设计系统 Mono Press

报刊编辑风格，token 定义在 [src/styles/global.css](src/styles/global.css) 的 `@theme` 中。颜色、圆角、字体都用 token，不写裸色值。

- 配色：纸本底 `paper` #f5f3ed、墨色 `ink` #14110d、次要文字 `stone`、分隔线 `line`。砖红 `clay` #a8281c 只用于危险操作、错误和低电量。
- 字体：标题用 `serif`（Noto Serif SC），界面用 `sans`（IBM Plex Sans），数字和代码用 `mono`（IBM Plex Mono）。
- 形态：所有圆角都是 0px（全局覆写，Radix 浮层除外）；1px 墨线；不用模糊。
- 页面 header：标题在上，meta 在下，不用眉题。
- canvas 无法读取 CSS 变量，帧预览的纸色和墨色在 `src/lib/eink/colors.ts` 里硬编码，改 token 时同步。

## 检查

```bash
bun run format:check   # bun run format 修复
bun run lint
bun run test
bun run build          # 含 tsc 类型检查
```

也可以在仓库根目录运行 `scripts/check.sh frontend`。
