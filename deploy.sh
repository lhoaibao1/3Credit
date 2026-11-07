#!/usr/bin/env bash
# 3Credit — cài đặt / cập nhật bằng một lệnh trên server Ubuntu/Debian.
#
#   curl -fsSL https://raw.githubusercontent.com/lhoaibao1/3Credit/main/deploy.sh | sudo bash
#
# Chạy với domain qua Cloudflare (bản ghi A, proxy cam) khi cổng 80 đã có website khác:
#   curl -fsSL …/deploy.sh | sudo DOMAIN=3credit.asia PORT=8080 bash
#   rồi tạo Cloudflare Origin Rule: Hostname ends with 3credit.asia -> Destination port 8080
#
# Chạy với domain qua Cloudflare Tunnel (HTTPS, không cần mở cổng):
#   curl -fsSL …/deploy.sh | sudo DOMAIN=3credit.asia CF_TUNNEL_TOKEN=<token> bash
#
# Chạy lại lệnh trên bất cứ lúc nào để cập nhật lên code mới nhất.
# File .env (mật khẩu, khóa mã hóa) được giữ nguyên giữa các lần cập nhật.
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/lhoaibao1/3Credit.git}"
BRANCH="${BRANCH:-main}"
APP_DIR="${APP_DIR:-/opt/3credit}"
WANT_PORT="${PORT:-}"   # cổng bạn chỉ định (nếu có)
WANT_DOMAIN="${DOMAIN:-}"
WANT_TOKEN="${CF_TUNNEL_TOKEN:-}"
FREE_PORT="${FREE_PORT:-0}"   # đã gỡ, giữ biến để báo cho người dùng

say()  { printf '\033[1;38;5;208m==>\033[0m %s\n' "$*"; }
fail() { printf '\033[1;31mLỗi:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "Hãy chạy với quyền root (thêm sudo)."

# 1. Công cụ cần thiết
if ! command -v git >/dev/null 2>&1 || ! command -v curl >/dev/null 2>&1; then
  say "Cài git, curl…"
  if command -v apt-get >/dev/null 2>&1; then
    apt-get update -qq && apt-get install -y -qq git curl ca-certificates openssl >/dev/null
  else
    fail "Không tìm thấy apt-get. Hãy tự cài git và curl rồi chạy lại."
  fi
fi
if ! command -v docker >/dev/null 2>&1; then
  say "Cài Docker…"
  curl -fsSL https://get.docker.com | sh
fi
systemctl enable --now docker >/dev/null 2>&1 || true
docker compose version >/dev/null 2>&1 || fail "Thiếu Docker Compose plugin (docker compose)."

# 2. Lấy code
if [ -d "$APP_DIR/.git" ]; then
  say "Cập nhật code tại $APP_DIR…"
  git -C "$APP_DIR" fetch --depth 1 origin "$BRANCH"
  git -C "$APP_DIR" reset --hard "origin/$BRANCH"
else
  say "Tải code về $APP_DIR…"
  git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$APP_DIR"
fi
cd "$APP_DIR"

# 3. Cấu hình (.env chỉ tạo một lần)
rand() { openssl rand -hex "$1" 2>/dev/null || head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }
PUBLIC_IP="$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null || hostname -I | awk '{print $1}')"
if [ ! -f .env ]; then
  say "Tạo file cấu hình .env…"
  ADMIN_PASSWORD="$(rand 9)"
  umask 077
  cat > .env <<ENV
APP_ENV=production
BASE_URL=http://$PUBLIC_IP
SESSION_SECRET=$(rand 32)
DATA_KEY=$(rand 32)
ADMIN_USER=admin
ADMIN_PASSWORD=$ADMIN_PASSWORD
PORT=80
ENV
  NEW_INSTALL=1
fi
setenv() { if grep -q "^$1=" .env; then sed -i "s|^$1=.*|$1=$2|" .env; else printf '%s=%s\n' "$1" "$2" >> .env; fi; }
[ -n "$WANT_DOMAIN" ] && setenv DOMAIN "$(echo "$WANT_DOMAIN" | tr 'A-Z' 'a-z' | sed -e 's|^https\?://||' -e 's|/.*$||')"
[ -n "$WANT_TOKEN" ] && setenv CF_TUNNEL_TOKEN "$WANT_TOKEN"
set -a; . ./.env; set +a
DOMAIN="${DOMAIN:-}"; CF_TUNNEL_TOKEN="${CF_TUNNEL_TOKEN:-}"
if [ -n "$CF_TUNNEL_TOKEN" ] && [ -z "$DOMAIN" ]; then fail "Đã có CF_TUNNEL_TOKEN nhưng thiếu DOMAIN (vd. DOMAIN=3credit.asia)."; fi
[ -n "$WANT_PORT" ] && PORT="$WANT_PORT"
PORT="${PORT:-80}"

# 4. Chọn cổng: nếu cổng đang bị chương trình khác chiếm thì tự chọn cổng trống
# Cổng mà container 3Credit đang dùng thì không tính là "bị chiếm" (không cần dừng web khi cập nhật)
OWN_PORT="$(docker port 3credit 8080/tcp 2>/dev/null | head -1 | awk -F: '{print $NF}')"
port_busy() {
  [ "$1" = "$OWN_PORT" ] && return 1
  if command -v ss >/dev/null 2>&1 && ss -ltnH 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]$1\$"; then return 0; fi
  (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null
}
# FREE_PORT đã bị gỡ: script không bao giờ tắt chương trình khác trên máy chủ
# (máy chủ có thể đang chạy website khác trên cổng 80).
if [ "$FREE_PORT" = 1 ]; then
  say "Bỏ qua FREE_PORT: script không tắt chương trình khác. Dùng Cloudflare Origin Rule để trỏ domain về cổng của 3Credit."
fi
if port_busy "$PORT"; then
  OWNER="$(ss -ltnpH 2>/dev/null | grep -E "[:.]$PORT " | grep -o 'users:(("[^"]*' | head -1 | cut -d'"' -f2)"
  say "Cổng $PORT đang được ${OWNER:-chương trình khác} dùng, tìm cổng trống khác…"
  NEWP=""
  for c in 8080 8081 8088 8888 3000 9000; do
    if ! port_busy "$c"; then NEWP="$c"; break; fi
  done
  [ -n "$NEWP" ] || fail "Không tìm được cổng trống. Chạy lại với PORT=<số cổng>."
  PORT="$NEWP"
fi
BASE_URL="http://$PUBLIC_IP"; [ "$PORT" = "80" ] || BASE_URL="$BASE_URL:$PORT"
if [ -n "$DOMAIN" ]; then
  BASE_URL="https://$DOMAIN"
  setenv TRUST_CF 1                       # lấy IP khách thật từ header của Cloudflare
fi
if [ -n "$CF_TUNNEL_TOKEN" ]; then
  setenv COMPOSE_PROFILES tunnel          # chạy thêm container cloudflared
  setenv BIND_ADDR 127.0.0.1              # web chỉ nghe nội bộ: không vào thẳng bằng IP được
  BIND_ADDR=127.0.0.1; COMPOSE_PROFILES=tunnel
else
  setenv COMPOSE_PROFILES ""
  setenv BIND_ADDR 0.0.0.0
  BIND_ADDR=0.0.0.0; COMPOSE_PROFILES=""
fi
setenv PORT "$PORT"; setenv BASE_URL "$BASE_URL"
export PORT BASE_URL BIND_ADDR COMPOSE_PROFILES

# 5. Mở cổng firewall nếu đang bật ufw
if [ "$BIND_ADDR" != "127.0.0.1" ] && command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow "$PORT"/tcp >/dev/null && say "Đã mở cổng $PORT trên ufw."
fi

# 6. Build và chạy
say "Build và khởi động (lần đầu mất vài phút)…"
# số phiên bản (commit) in vào binary: xem tại <domain>/version và ở chân menu CMS
export GIT_SHA="$(git -C "$APP_DIR" rev-parse --short HEAD 2>/dev/null || echo dev)"
docker compose up -d --build --remove-orphans

say "Chờ server sẵn sàng…"
# 1) app trong container đã chạy chưa (không phụ thuộc mạng của máy chủ)
for _ in $(seq 1 30); do
  if docker exec 3credit wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1; then OK=1; break; fi
  sleep 2
done
[ "${OK:-0}" = 1 ] || { docker compose logs --tail 50; fail "Ứng dụng không khởi động được. Xem log ở trên."; }
docker image prune -f >/dev/null 2>&1 || true

# 2) cổng trên máy chủ có thông vào container không
HOST_OK=0
for _ in $(seq 1 10); do
  if curl -fsS --max-time 3 "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1 \
     || curl -fsS --max-time 3 "http://$PUBLIC_IP:$PORT/healthz" >/dev/null 2>&1; then HOST_OK=1; break; fi
  sleep 2
done
if [ "$HOST_OK" != 1 ]; then
  echo
  say "Ứng dụng đã chạy trong container, nhưng chưa gọi được qua cổng $PORT trên máy chủ."
  echo "    Thông tin để kiểm tra (gửi phần này cho người hỗ trợ):"
  echo "    --- docker ps ---";   docker ps --filter name=3credit --format '    {{.Names}}  {{.Status}}  {{.Ports}}'
  echo "    --- cổng $PORT ---"; (ss -ltnp 2>/dev/null || netstat -ltnp 2>/dev/null) | grep -E "[:.]$PORT " | sed 's/^/    /' || echo "    (không thấy tiến trình nào lắng nghe cổng $PORT)"
  echo "    --- curl ---";       curl -sS -o /dev/null -w "    127.0.0.1:$PORT -> HTTP %{http_code}\n" --max-time 3 "http://127.0.0.1:$PORT/healthz" 2>&1 | sed 's/^/    /'
  echo "    --- .env ---";       grep -E '^(PORT|BASE_URL)=' .env | sed 's/^/    /'
  echo
  echo "    Hãy thử mở $BASE_URL bằng trình duyệt. Nếu không vào được, kiểm tra firewall"
  echo "    của nhà cung cấp cloud đã mở cổng $PORT (TCP) chưa."
fi

# 7. Tự cập nhật khi GitHub có commit mới (tắt bằng AUTO_UPDATE=0)
AUTO_UPDATE="${AUTO_UPDATE:-1}"
UPDATE_EVERY="${UPDATE_EVERY:-2min}"
if [ "$AUTO_UPDATE" = 1 ] && command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  chmod +x "$APP_DIR/update.sh"
  cat > /etc/systemd/system/3credit-update.service <<UNIT
[Unit]
Description=3Credit: cập nhật khi GitHub có commit mới
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=oneshot
Environment=APP_DIR=$APP_DIR
Environment=BRANCH=$BRANCH
ExecStart=$APP_DIR/update.sh
TimeoutStartSec=1800
UNIT
  cat > /etc/systemd/system/3credit-update.timer <<UNIT
[Unit]
Description=3Credit: kiểm tra cập nhật mỗi $UPDATE_EVERY

[Timer]
OnBootSec=2min
OnUnitInactiveSec=$UPDATE_EVERY
Persistent=true

[Install]
WantedBy=timers.target
UNIT
  systemctl daemon-reload
  systemctl enable --now 3credit-update.timer >/dev/null 2>&1
  AUTO_MSG="bật (kiểm tra GitHub mỗi $UPDATE_EVERY)"
elif [ "$AUTO_UPDATE" = 0 ] && command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now 3credit-update.timer >/dev/null 2>&1 || true
  AUTO_MSG="tắt"
else
  AUTO_MSG="không hỗ trợ (máy không dùng systemd)"
fi

URL="$BASE_URL"
if [ -n "$CF_TUNNEL_TOKEN" ]; then
  sleep 3
  if docker compose logs --tail 40 cloudflared 2>/dev/null | grep -qi "Registered tunnel connection"; then
    TUNNEL_MSG="đã kết nối Cloudflare"
  else
    TUNNEL_MSG="CHƯA kết nối được – xem: cd $APP_DIR && docker compose logs cloudflared"
  fi
fi
echo
say "3Credit đã chạy!"
echo "    Trang khách : $URL"
if [ -n "$DOMAIN" ]; then CMS_URL="https://cms.$DOMAIN"; else CMS_URL="$URL"; fi
echo "    CMS         : $CMS_URL/login"
[ -n "$CF_TUNNEL_TOKEN" ] && echo "    Tunnel      : $TUNNEL_MSG"
echo "    Lần đầu vào CMS: tự tạo tài khoản tại $CMS_URL/setup"
echo "                     Mã thiết lập = ADMIN_PASSWORD trong $APP_DIR/.env:"
echo "                     $(grep -E '^ADMIN_PASSWORD=' "$APP_DIR/.env" | cut -d= -f2-)"
echo
echo "    Tự cập nhật: $AUTO_MSG"
echo "    Lịch sử cập nhật: journalctl -u 3credit-update -n 50"
echo "    Cập nhật ngay   : sudo $APP_DIR/update.sh"
echo "    Xem log   : cd $APP_DIR && docker compose logs -f"
echo "    Dừng      : cd $APP_DIR && docker compose down"
echo
[ -n "$DOMAIN" ] || echo "    Lưu ý: đang chạy HTTP qua IP, chưa mã hóa. Khi có domain hãy bật HTTPS trước khi nhận khách thật."
