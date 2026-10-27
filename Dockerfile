# syntax=docker/dockerfile:1

# 前端和 Go 在构建机原生架构上构建：前端产物与架构无关，Go 交叉编译出静态二进制。
# ffmpeg 在目标架构上从源码编译，只包含音频转码用到的格式。

FROM --platform=$BUILDPLATFORM oven/bun:1.4.3-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile
COPY frontend ./
RUN bun run build

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/slate ./cmd/slate \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go test -c -o /out/media.test ./internal/media

# 输入：上传表单接受的 wav/mp3/aac/m4a/ogg/opus/flac/wma，以及 TTS 返回的 s16le；输出：s16le。
FROM alpine:3.23 AS ffmpeg
ARG FFMPEG_VERSION=9.0.2
ARG FFMPEG_SHA256=8c3850283eb25fa026482078a04051e0be17347b09ef81a0849bec15a96e002e
RUN apk add --no-cache build-base nasm curl xz
WORKDIR /src
RUN curl -fsSLo ffmpeg.tar.xz https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz \
 && echo "${FFMPEG_SHA256}  ffmpeg.tar.xz" | sha256sum -c - \
 && tar -xJf ffmpeg.tar.xz --strip-components=1 \
 && ./configure \
      --disable-everything --disable-autodetect --disable-network \
      --disable-doc --disable-debug --disable-ffplay --disable-ffprobe \
      --disable-avdevice --disable-swscale \
      --enable-static --disable-shared --extra-ldflags=-static \
      --enable-protocol=pipe \
      --enable-demuxer=wav,mp3,aac,mov,ogg,flac,asf,pcm_s16le \
      --enable-parser=mpegaudio,aac,flac,vorbis,opus \
      --enable-decoder=pcm_s16le,pcm_s16be,pcm_s24le,pcm_s32le,pcm_u8,pcm_f32le,pcm_f64le,pcm_alaw,pcm_mulaw,adpcm_ms,adpcm_ima_wav \
      --enable-decoder=mp3,mp3float,aac,alac,vorbis,opus,flac,wmav1,wmav2,wmapro \
      --enable-encoder=pcm_s16le --enable-muxer=pcm_s16le \
      --enable-swresample --enable-filter=aresample,aformat,anull,atrim \
 && make -j"$(nproc)" ffmpeg \
 && strip ffmpeg

# 用刚编出的 ffmpeg 跑转码测试，漏掉格式时构建失败。
FROM ffmpeg AS ffmpeg-check
COPY --from=backend /out/media.test ./
COPY backend/internal/media/testdata ./testdata
RUN PATH=/src:$PATH ./media.test -test.v

FROM alpine:3.23
RUN apk add --no-cache ca-certificates \
 && adduser -D -u 1000 slate \
 && mkdir -p /data/blobs && chown -R slate:slate /data
WORKDIR /app
COPY --from=ffmpeg-check /src/ffmpeg /usr/local/bin/ffmpeg
COPY --from=frontend /src/frontend/dist ./public
COPY --from=backend /out/slate /usr/local/bin/slate
ENV PORT=9494 BLOB_DIR=/data/blobs PUBLIC_DIR=/app/public
USER slate
EXPOSE 9494
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --spider -q "http://127.0.0.1:${PORT}/api/health" || exit 1
ENTRYPOINT ["slate"]
CMD ["serve"]
