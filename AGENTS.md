# Agent Guide

给在本仓库工作的 AI 代理。通用开发约定与发版流程见 [CONTRIBUTING.md](CONTRIBUTING.md)，这里只列必须额外遵守的规则。

## 工作方式

- 默认在 `master` 上开发。不要回滚用户已有改动；无关的脏工作区直接忽略。
- 改动前先读对应模块的 README（`backend/`、`frontend/`、`shared/`、`firmware/`）。
- 前端 UI 改动遵守 `frontend/README.md` 的 Mono Press 设计系统。
- 固件改动涉及分区表、NVS、同步协议或 OTA 时，在最终说明里写明风险和验证结果。
- 大改后运行 CONTRIBUTING.md 中的提交前检查。

## 提交

- 格式见 CONTRIBUTING.md。带 body 时用单个 `-m`：`git commit -m $'subject\n\n- item'`。
- 提交信息里不加任何 AI 署名、生成标识或 Co-Authored-By trailer。

## 发版

按 CONTRIBUTING.md「发布版本」执行，另外：

- 只用 annotated tag；Release notes 只来自 tag body，不手动编辑 GitHub Release。
- 不在 release workflow 之外推送 `vX.Y.Z` 镜像 tag。
- 不重跑旧版本 tag。
