#!/usr/bin/env bash
# 中文：把任意常见视频转成浏览器可播的 H.264 + AAC MP4（含 faststart）。
#       适用于 mpeg4/mp4v 等「只有声音没有画面」的文件；默认输出名为 *.h264.mp4。
# English: Convert common video files to browser-playable H.264 + AAC MP4 (with faststart).
#          Use for mpeg4/mp4v sources that play audio-only in HTML5 <video>.
#          Default output: <input>.h264.mp4
# 用法 / Usage: to_h264.sh <input> [output.mp4]
set -euo pipefail

in="${1:?用法 / Usage: $0 <input> [output.mp4]}"
out="${2:-${in%.*}.h264.mp4}"

ffmpeg -nostdin -hide_banner -y -i "$in" \
  -map 0:v:0 -map 0:a:0? \
  -c:v libx264 -preset medium -pix_fmt yuv420p -crf 20 \
  -c:a aac -profile:a aac_low -ac 2 -ar 44100 -b:a 192k \
  -movflags +faststart \
  "$out"

echo "完成 / Done: $out"
