#!/bin/sh
set -eu

VIDEO_DIR="${VIDEOHUB_VIDEO_DIR:-/videos}"
PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -u)" = "0" ]; then
  mkdir -p "$VIDEO_DIR"

  if ! chown "$PUID:$PGID" "$VIDEO_DIR" 2>/dev/null; then
    chmod 777 "$VIDEO_DIR" 2>/dev/null || true
  fi

  exec su-exec "$PUID:$PGID" /app/videohub "$@"
fi

exec /app/videohub "$@"
