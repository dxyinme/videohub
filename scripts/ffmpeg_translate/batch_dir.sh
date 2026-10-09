#!/usr/bin/env bash
# 中文：批量转换某目录下的视频。默认调用 to_web.sh（网页友好）；
#       第二个参数可改为 to_h264（兼容浏览器编码，不限峰值码率）。
#       会跳过已生成的 *.h264.mp4 / *.web.mp4。
# English: Batch-convert videos in a directory. Default worker: to_web.sh.
#          Pass to_h264 as 2nd arg for unconstrained H.264 convert.
#          Skips already-produced *.h264.mp4 / *.web.mp4.
# 用法 / Usage: batch_dir.sh <dir> [to_web|to_h264]
set -euo pipefail

dir="${1:?用法 / Usage: $0 <dir> [to_web|to_h264]}"
mode="${2:-to_web}"
here="$(cd "$(dirname "$0")" && pwd)"
worker="$here/${mode}.sh"

if [[ ! -f "$worker" ]]; then
  echo "未知模式 / unknown mode: $mode（可用 / allowed: to_web, to_h264）" >&2
  exit 1
fi

shopt -s nullglob nocaseglob
for f in "$dir"/*.{mp4,m4v,mkv,mov,avi,rmvb,rm,webm}; do
  [[ -f "$f" ]] || continue
  case "$f" in
    *.h264.mp4|*.web.mp4) echo "跳过已转换 / skip: $f"; continue ;;
  esac
  echo "==> $f"
  bash "$worker" "$f"
done
