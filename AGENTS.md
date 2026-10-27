# Agent Guide

给在本仓库工作的 AI 代理。通用开发约定与发版流程见 [CONTRIBUTING.md](CONTRIBUTING.md)，这里只列必须额外遵守的规则。

## 工作方式

- 默认在 `master` 上开发。不要回滚用户已有改动；无关的脏工作区直接忽略。
- 改动前先读对应模块的 README（`backend/`、`frontend/`、`firmware/`）。
- 前端 UI 改动遵守 `frontend/README.md` 的 Mono Press 设计系统。
- 改完运行 `scripts/check.sh`（或只跑改动的一端：`scripts/check.sh backend|frontend`）。

## 契约与生成代码

- 前后端契约只有一份：`backend/internal/api`。改了它、`backend/internal/store/sql` 或 `backend/migrations` 后运行 `go generate ./...`（在 `backend/` 下），生成物一起提交；不要手改 `frontend/src/api/generated.ts`、`catalog.json` 和 `backend/internal/store/db`。
- 已应用的迁移文件不能修改，schema 变化写新的迁移文件。
- 渲染布局变化后用 `go test ./internal/render -update` 更新 golden PNG，并在说明里列出变化的帧。
- 设备协议改动要同时改 `backend/internal/api/device.go` 和 `firmware/main/sync/api_client.*`；不兼容时把 `api.DeviceProtocol` 与固件的 `api::kProtocol` 一起加一。

## 固件

- 改动涉及分区表、NVS、同步协议、缓存布局或 OTA 时，在最终说明里写明风险和验证结果（编译、联调、实机分别验证了什么）。
- 改了 LittleFS 缓存格式就递增 `storage/cache/cache.cc` 的 `kLayoutVersion`，旧缓存会在启动时清空重下。
- ESP-IDF 环境由 `firmware/.envrc`（本地文件，不入库）通过 direnv 加载，在 `firmware/` 下直接运行 `idf.py`。`idf.py monitor` 是交互式命令，agent 会话中用本机的 `esp-log <秒数>` 抓串口日志。

## 提交

- 格式见 CONTRIBUTING.md。带 body 时用单个 `-m`：`git commit -m $'subject\n\n- item'`。
- 提交信息里不加任何 AI 署名、生成标识或 Co-Authored-By trailer。

## 发版

按 CONTRIBUTING.md「发布版本」执行，另外：

- 只用 annotated tag；Release notes 只来自 tag body，不手动编辑 GitHub Release。
- 不在 release workflow 之外推送 `vX.Y.Z` 镜像 tag。
- 不重跑旧版本 tag。
