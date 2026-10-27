#!/usr/bin/env bash
# 生成后端渲染用的点阵 JSON 字体（backend/internal/render/assets/fonts）。
# 上游字体按固定 release 下载；Zfull-GB 用同目录的 TTF，ZFULL_SIZES 控制字号。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PROBE_DIR="${SLATE_FONT_PROBE_DIR:-${TMPDIR:-/tmp}/slate-font-probe}"
WORK_DIR="${SLATE_FONT_WORK_DIR:-${TMPDIR:-/tmp}/slate-font-test}"
OUT_DIR="$ROOT/backend/internal/render/assets/fonts"
GENFONT=(go -C "$ROOT/backend" run ./cmd/genfont)
FIRMWARE_FONT_DIR="$ROOT/firmware/main/resources/fonts"
SOURCE_HAN_SANS_C="${SLATE_SOURCE_HAN_SANS_C:-$FIRMWARE_FONT_DIR/source_han_sans_sc_regular_slim.c}"
SYMBOLS="墨水屏字体测试中文点阵简繁日"
DEMO_SYMBOLS="墨水屏字体测试中文点阵今日天气多云风力级简繁标点，。！？；：～一二三四五六七八九十口日目田回黑白像素横竖撇捺线面黑墨屏°"
ZFULL_TTF="${ZFULL_TTF:-$ROOT/scripts/fonts/Zfull-GB.ttf}"
ZFULL_SIZES="${ZFULL_SIZES:-10 12 14 16 18}"
LV_FONT_CONV_BIN="$(command -v lv_font_conv || true)"

mkdir -p "$PROBE_DIR" "$WORK_DIR" "$OUT_DIR"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

convert_ttf() {
  local src="$1"
  local size="$2"
  local name="$3"
  local out="$4"
  local extra="${5:-$SYMBOLS}"
  local c_file="$WORK_DIR/$out.c"
  if [[ -n "$extra" ]]; then
    node --stack-size=65500 "$LV_FONT_CONV_BIN" --no-compress --bpp 1 --size "$size" --font "$src" --autohint-off \
      -r 0x20-0x7F --symbols "$extra" --format lvgl -o "$c_file" --lv-font-name "$name"
  else
    node --stack-size=65500 "$LV_FONT_CONV_BIN" --no-compress --bpp 1 --size "$size" --font "$src" --autohint-off \
      -r 0x20-0x7F --format lvgl -o "$c_file" --lv-font-name "$name"
  fi
  "${GENFONT[@]}" lvgl "$c_file" "$OUT_DIR/$out.json"
}

convert_ttf_full() {
  local src="$1"
  local size="$2"
  local name="$3"
  local out="$4"
  local c_file="$WORK_DIR/$out.c"
  local ranges
  # fc-query prints hex ranges like "20-7e a0"; lv_font_conv wants 0x20-0x7e,0xa0.
  ranges="$(fc-query --format='%{charset}' "$src" | tr -s ' ' '\n' | sed -E '/^$/d; s/^/0x/; s/-/-0x/' | paste -sd, -)"
  node --stack-size=65500 "$LV_FONT_CONV_BIN" --no-compress --bpp 1 --size "$size" --font "$src" --autohint-off \
    -r "$ranges" --format lvgl -o "$c_file" --lv-font-name "$name"
  "${GENFONT[@]}" lvgl "$c_file" "$OUT_DIR/$out.json"
}

convert_ttf_demo() {
  convert_ttf "$1" "$2" "$3" "$4" "$DEMO_SYMBOLS"
}

download_release() {
  local repo="$1"
  local tag="$2"
  local dir="$3"
  shift 3
  mkdir -p "$dir"
  gh release download "$tag" --repo "$repo" --dir "$dir" --clobber "$@"
}

download_raw() {
  local repo="$1"
  local path="$2"
  local out="$3"
  mkdir -p "$(dirname "$out")"
  gh api -H "Accept: application/vnd.github.raw" "repos/$repo/contents/$path" > "$out"
}

need go
need gh
need node
need python3
need fc-query
need lv_font_conv
need unzip
need tar

download_release TakWolf/fusion-pixel-font 2026.05.07 "$PROBE_DIR/fusion10" --pattern "fusion-pixel-font-10px-proportional-ttf-*.zip"
download_release TakWolf/fusion-pixel-font 2026.05.07 "$PROBE_DIR/fusion12" --pattern "fusion-pixel-font-12px-proportional-ttf-*.zip"
download_release TakWolf/ark-pixel-font 2026.05.07 "$PROBE_DIR/ark10" --pattern "ark-pixel-font-10px-proportional-ttf-*.zip"
download_release TakWolf/ark-pixel-font 2026.05.07 "$PROBE_DIR/ark12" --pattern "ark-pixel-font-12px-proportional-ttf-*.zip"
download_release TakWolf/ark-pixel-font 2026.05.07 "$PROBE_DIR/ark16" --pattern "ark-pixel-font-16px-proportional-ttf-*.zip"
download_release fcambus/spleen 2.2.0 "$PROBE_DIR/spleen" --pattern "spleen-2.2.0.tar.gz"
download_release the-moonwitch/Cozette v.1.30.0 "$PROBE_DIR/cozette" --pattern "CozetteVector.ttf"
download_release multitheftauto/unifont v16.0.04 "$PROBE_DIR/unifont" --pattern "unifont-16.0.04.otf"
download_release Astro-2539/ZLabs-Pixel-12px Build_20260519 "$PROBE_DIR/zlabs-pixel" --pattern "ZLabsPixel_12px_M_CN.ttf"
download_release Astro-2539/ZLabs-RoundPix-12px Build_20260508 "$PROBE_DIR/zlabs-round12" --pattern "ZLabsRoundPix_12px_M_CN.ttf"
download_release Astro-2539/ZLabs-RoundPix-16px Build_20260501 "$PROBE_DIR/zlabs-round16" --pattern "ZLabsRoundPix_16px_M_CN.ttf"
download_release Warren2060/ChillBitmap v2.502 "$PROBE_DIR/chill" --pattern "ChillBitmap_New.zip"
download_raw ACh-K/Cubic-11 fonts/ttf/Cubic_11.ttf "$PROBE_DIR/cubic/Cubic_11.ttf"

unzip -o -q "$PROBE_DIR/fusion10"/fusion-pixel-font-10px-proportional-ttf-*.zip -d "$PROBE_DIR/fusion10"
unzip -o -q "$PROBE_DIR/fusion12"/fusion-pixel-font-12px-proportional-ttf-*.zip -d "$PROBE_DIR/fusion12"
unzip -o -q "$PROBE_DIR/ark10"/ark-pixel-font-10px-proportional-ttf-*.zip -d "$PROBE_DIR/ark10"
unzip -o -q "$PROBE_DIR/ark12"/ark-pixel-font-12px-proportional-ttf-*.zip -d "$PROBE_DIR/ark12"
unzip -o -q "$PROBE_DIR/ark16"/ark-pixel-font-16px-proportional-ttf-*.zip -d "$PROBE_DIR/ark16"
unzip -o -q "$PROBE_DIR/chill/ChillBitmap_New.zip" -d "$PROBE_DIR/chill"
tar -xzf "$PROBE_DIR/spleen/spleen-2.2.0.tar.gz" -C "$PROBE_DIR/spleen"

convert_ttf_full "$PROBE_DIR/fusion10/fusion-pixel-10px-proportional-zh_hans.ttf" 10 FusionPixel_10 fusion-pixel-10
convert_ttf_full "$PROBE_DIR/fusion12/fusion-pixel-12px-proportional-zh_hans.ttf" 12 FusionPixel_12 fusion-pixel-12
convert_ttf_full "$PROBE_DIR/ark10/ark-pixel-10px-proportional-zh_cn.ttf" 10 ArkPixel_10 ark-pixel-10
convert_ttf_full "$PROBE_DIR/ark12/ark-pixel-12px-proportional-zh_cn.ttf" 12 ArkPixel_12 ark-pixel-12
convert_ttf_full "$PROBE_DIR/ark16/ark-pixel-16px-proportional-zh_cn.ttf" 16 ArkPixel_16 ark-pixel-16
convert_ttf_full "$PROBE_DIR/unifont/unifont-16.0.04.otf" 16 Unifont_16 unifont-16
convert_ttf_full "$PROBE_DIR/cozette/CozetteVector.ttf" 13 Cozette_13 cozette-13
convert_ttf_full "$ROOT/firmware/managed_components/lvgl__lvgl/scripts/built_in_font/Montserrat-Medium.ttf" 48 Montserrat_48 montserrat-48
convert_ttf_full "$ROOT/firmware/managed_components/lvgl__lvgl/scripts/built_in_font/FontAwesome5-Solid+Brands+Regular.woff" 14 FontAwesome_14 font-awesome-14
convert_ttf_full "$ROOT/firmware/managed_components/lvgl__lvgl/scripts/built_in_font/FontAwesome5-Solid+Brands+Regular.woff" 30 FontAwesome_30 font-awesome-30
convert_ttf_demo "$PROBE_DIR/zlabs-pixel/ZLabsPixel_12px_M_CN.ttf" 12 ZLabsPixel_12_Demo zlabs-pixel-12-demo
convert_ttf_demo "$PROBE_DIR/zlabs-round12/ZLabsRoundPix_12px_M_CN.ttf" 12 ZLabsRoundPix_12_Demo zlabs-roundpix-12-demo
convert_ttf_demo "$PROBE_DIR/zlabs-round16/ZLabsRoundPix_16px_M_CN.ttf" 16 ZLabsRoundPix_16_Demo zlabs-roundpix-16-demo
convert_ttf_demo "$PROBE_DIR/chill/ChillBitmap_New/ChillBitmap_16px.ttf" 16 ChillBitmap_16_Demo chill-bitmap-16-demo
convert_ttf_demo "$PROBE_DIR/cubic/Cubic_11.ttf" 11 Cubic_11_Demo cubic-11-demo

for item in 6x12:12 8x16:16 12x24:24 16x32:32 32x64:64; do
  name="${item%%:*}"
  size="${item##*:}"
  convert_ttf_full "$PROBE_DIR/spleen/spleen-2.2.0/spleen-$name.otf" "$size" "Spleen_${name//x/_}" "spleen-$name"
done

"${GENFONT[@]}" bdf "$PROBE_DIR/spleen/spleen-2.2.0/spleen-5x8.bdf" "$OUT_DIR/spleen-5x8.json"
if [[ -f "$SOURCE_HAN_SANS_C" ]]; then
  "${GENFONT[@]}" lvgl "$SOURCE_HAN_SANS_C" "$OUT_DIR/source-han-sans-16-slim.json"
elif [[ -f "$OUT_DIR/source-han-sans-16-slim.json" ]]; then
  echo "warning: $SOURCE_HAN_SANS_C not found; keeping existing source-han-sans-16-slim.json" >&2
else
  echo "missing source Han Sans LVGL font: $SOURCE_HAN_SANS_C" >&2
  echo "set SLATE_SOURCE_HAN_SANS_C to a repo-local or absolute LVGL .c font source" >&2
  exit 1
fi

# GB2312 全部 6763 个汉字加常用符号。
zfull_symbols="$(python3 - <<'PY2'
import sys

BASE_SYMBOLS = (
    r""" !"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\]^_`abcdefghijklmnopqrstuvwxyz{|}~"""
    "，。！？：；、（）《》【】“”‘’—…·"
    "☀☁☂☃⚡❄"
    "¥€£₿¢"
    "←↑→↓↖↗↘↙↔↕➜➝➞➤"
    "℃℉°‰"
    "±×÷≈≠≤≥∞√"
    "～–「」『』〈〉〔〕〖〗"
    "©®™§¶"
)

hanzi = []
for lead in range(0xB0, 0xF8):
    for trail in range(0xA1, 0xFF):
        try:
            hanzi.append(bytes((lead, trail)).decode("gb2312"))
        except UnicodeDecodeError:
            pass

if len(hanzi) != 6763:
    raise SystemExit(f"expected 6763 GB2312 Hanzi, got {len(hanzi)}")

sys.stdout.write(BASE_SYMBOLS + "".join(hanzi))
PY2
)"

for size in $ZFULL_SIZES; do
  node --stack-size=65500 "$LV_FONT_CONV_BIN" --no-compress --bpp 1 --size "$size" --font "$ZFULL_TTF" --autohint-off \
    --symbols "$zfull_symbols" --format lvgl -o "$WORK_DIR/zfull-gb-$size.c" --lv-font-name "ZfullGB_$size"
  "${GENFONT[@]}" lvgl "$WORK_DIR/zfull-gb-$size.c" "$OUT_DIR/zfull-gb-$size.json"
done
