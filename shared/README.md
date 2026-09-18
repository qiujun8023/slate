# Slate / Shared

前后端共用的 TypeScript 源码：zod schema、动态内容配置、抖动与图像预处理。前后端直接 import `shared/src`，不需要先构建。

```text
src/
├── api.ts                 API 前缀、帧与音频常量、登录注册 schema、错误结构
├── types/                 设备、内容组、内容的请求与响应 schema
├── dynamic/
│   ├── config.ts          动态内容类型与各类型配置、TTS 音色
│   ├── templates.ts       dashboard 模板 schema 与内置模板
│   ├── hot-list-sources.ts 热榜源列表与旧 ID 映射
│   └── fonts.ts           字体测试的字体列表
├── dither.ts              1bpp 抖动
└── preprocess.ts          灰度化、自动反相、自动对比度
```

## 约定

- schema 用 `PascalCase`，对应类型加 `T` 后缀：`PollRequest` / `PollRequestT`。后端 DTO 通过 `static schema = PollRequest` 接入全局 `ZodValidationPipe`。
- 帧为 400×300 packed 1bpp，共 15000 字节，MSB 在前，bit=1 为白、bit=0 为黑，与固件一致。
- 音频为 16 kHz、单声道、16-bit PCM。

## 动态内容

`DynamicConfig` 是以 `type` 区分的 union，共 9 种类型（见 `DynamicType`）。支持音频的类型可设置 `audio_enabled` / `audio_voice`，`isAudioDynamicConfig()` 用来判断。周期刷新的类型可设置 `refresh_interval_sec`，各类型的取值范围不同。

dashboard 模板的画布为 400×300，`y` 最小 24（顶部留给固件状态栏），最多 32 个区块，区块类型有 `text` / `metric` / `progress` / `sparkline` / `line` / `rect`。推送数据的格式是 `{ version: 1, data: {...} }`，`data` 至少包含一个字段。

热榜源的 ID 改名后，要在 `normalizeHotListSourceId()` 里保留旧 ID 的映射，避免已保存的配置失效。实际抓取逻辑在 `backend/src/modules/hot-list/`。

## 图像处理

前端预览和后端渲染必须按相同顺序处理，否则预览会和设备显示不一致：

1. `rgbaToGray`：Rec.709 系数。
2. `autoInvert`：四角偏暗时判定为黑底，整体反相。
3. `autoContrast(1)`：两端各裁掉 1% 后拉伸，等同 PIL `ImageOps.autocontrast`。
4. `ditherToBinary`（每像素 0/255，供 canvas 预览）或 `ditherTo1bpp`（packed，供设备使用）。

抖动默认在线性光下进行（`gammaCorrect`），误差扩散默认蛇形扫描（`serpentine`）。前端新建图片默认用 `floyd`；API 未指定时用 `threshold`，保证老调用方的输出不变。
