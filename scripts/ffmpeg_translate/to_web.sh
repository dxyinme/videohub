#!/usr/bin/env bash
# 中文：压成更适合网页 Range 直出的 MP4——限制峰值码率约 4Mbps，关键帧约 2 秒，
#       减轻高码率长 GOP 导致的播放卡顿；默认输出名为 *.web.mp4。
# English: Re-encode for smoother HTML5 progressive / Range playback: ~4 Mbps maxrate,
#          ~2s keyframes (GOP), LC-AAC. Helps with high-bitrate / long-GOP stutter.
#          Default output: <input>.web.mp4
# 用法 / Usage: to_web.sh <input> [output.mp4]
set -euo pipefail

in="${1:?用法 / Usage: $0 <input> [output.mp4]}"
out="${2:-${in%.*}.web.mp4}"

ffmpeg -nostdin -hide_banner -y -i "$in" \
  -map 0:v:0 -map 0:a:0? \
  -c:v libx264 -preset medium -pix_fmt yuv420p \
  -crf 20 -maxrate 4M -bufsize 8M \
  -g 50 -keyint_min 50 -sc_threshold 0 \
  -c:a aac -profile:a aac_low -ac 2 -ar 44100 -b:a 160k \
  -movflags +faststart \
  "$out"

echo "完成 / Done: $out"
