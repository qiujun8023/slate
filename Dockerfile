# syntax=docker/dockerfile:1

# ---------- builder(只用来构 frontend dist;backend 走解释执行,不预编译) ----------
FROM oven/bun:1-slim AS builder

WORKDIR /app

# 先拷依赖描述,利用层缓存(workspaces 需要每个子包的 package.json)
COPY package.json bun.lock ./
COPY backend/package.json ./backend/
COPY frontend/package.json ./frontend/
COPY shared/package.json ./shared/

RUN bun install --frozen-lockfile

# 只拷构 frontend 需要的源码 → vite build
COPY frontend ./frontend
COPY shared ./shared
RUN bun run --cwd frontend build

# ---------- runner ----------
FROM oven/bun:1-alpine AS runner

WORKDIR /app

ENV NODE_ENV=production \
    PORT=9494 \
    BLOB_DIR=/data/blobs

# 依赖：
#   - ffmpeg：音频转码（audio.service 把上传音频转成 16k mono s16le PCM）
# HEALTHCHECK 复用 alpine 自带的 busybox wget,无需额外装包
# 字体和位图字库由 backend/assets/ 目录随代码库入库，不依赖系统 fontconfig。
RUN apk add --no-cache ffmpeg

# === 按变更频率从低到高分层,客户端 docker pull 增量最小 ===

# 1. 包描述 + lockfile:仅 deps 变化时失效
COPY package.json bun.lock ./
COPY backend/package.json ./backend/
COPY frontend/package.json ./frontend/
COPY shared/package.json ./shared/

# 2. 仅生产依赖(--production 跳过所有 devDependencies),独占一层 → 代码改动不会让它失效
RUN bun install --frozen-lockfile --production

# 3. Prisma schema + 生成 client(generate 产物落在 node_modules 内,
#    单独成层 → 仅 schema 变更才失效)。entrypoint 还要用 prisma migrate deploy,
#    prisma CLI 已升为 backend dependency,prod install 已带上。
COPY backend/prisma ./backend/prisma
COPY backend/prisma.config.ts ./backend/
RUN cd backend && bunx prisma generate

# 4. 前端构建产物(前端代码变才失效)
COPY --from=builder /app/frontend/dist ./frontend/dist

# 5. shared 源码(很少变)
COPY shared/src ./shared/src

# 6. backend 配置(几乎不变)
COPY backend/tsconfig.json ./backend/

# 7. 运行时资产（矢量字体 + 1bpp 位图字库，变更极少，单独成层）
COPY backend/assets ./backend/assets

# 8. backend 脚本（可选临时 job、调试和一次性维护任务）
COPY backend/scripts ./backend/scripts

# 9. backend 源码(改动最频繁,~300K,放最后让其他层全部命中缓存)
COPY backend/src ./backend/src

# 10. entrypoint
COPY entrypoint.sh ./
RUN chmod +x /app/entrypoint.sh

USER bun

EXPOSE 9494

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
  CMD [ "${SLATE_RUN_MODE:-server}" != "server" ] || wget --spider -q "http://127.0.0.1:${PORT}/healthz" || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
