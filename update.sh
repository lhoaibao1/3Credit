#!/usr/bin/env bash
# 3Credit — tự cập nhật khi nhánh trên GitHub có commit mới.
# Được systemd timer (3credit-update.timer) gọi định kỳ; cũng có thể chạy tay:
#   sudo /opt/3credit/update.sh
set -euo pipefail

APP_DIR="${APP_DIR:-/opt/3credit}"
BRANCH="${BRANCH:-main}"
LOCK=/run/3credit-update.lock

exec 9>"$LOCK"
flock -n 9 || { echo "Đang có một lần cập nhật khác chạy, bỏ qua."; exit 0; }

cd "$APP_DIR"
git fetch --quiet --depth 1 origin "$BRANCH"
LOCAL="$(git rev-parse HEAD)"
REMOTE="$(git rev-parse "origin/$BRANCH")"
if [ "$LOCAL" = "$REMOTE" ]; then
  exit 0
fi
FAILED_FILE="$APP_DIR/.update-failed"
if [ -f "$FAILED_FILE" ] && [ "$(cat "$FAILED_FILE")" = "$REMOTE" ]; then
  exit 0   # bản này đã cập nhật lỗi; chờ commit mới hơn
fi

echo "Có bản mới: ${LOCAL:0:7} -> ${REMOTE:0:7} ($(git log -1 --format=%s "origin/$BRANCH"))"
# Chạy deploy.sh từ bản sao tạm: deploy.sh sẽ cập nhật chính file trong repo khi đang chạy.
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
git show "origin/$BRANCH:deploy.sh" > "$TMP"
if APP_DIR="$APP_DIR" BRANCH="$BRANCH" bash "$TMP"; then
  rm -f "$FAILED_FILE"
  echo "Đã cập nhật lên ${REMOTE:0:7}."
else
  echo "$REMOTE" > "$FAILED_FILE"
  git reset --quiet --hard "$LOCAL"   # đưa code về đúng bản đang chạy
  echo "Cập nhật lên ${REMOTE:0:7} THẤT BẠI. Web vẫn chạy bản ${LOCAL:0:7}; sẽ thử lại khi có commit mới." >&2
  exit 1
fi
