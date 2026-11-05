# 3Credit — Financial Offer Marketplace (Go)

Nền tảng so sánh, đề xuất và phân phối sản phẩm tài chính. 3Credit **không** trực tiếp cho vay.

## Chạy thử

Cần Go 1.24 trở lên. Không cần CGO, database là SQLite (driver thuần Go).

```bash
ADMIN_USER=admin ADMIN_PASSWORD=doimatkhau go run ./cmd/server
```

Mở <http://localhost:8080> (trang khách) và <http://localhost:8080/admin> (quản trị).

Lần chạy đầu, server tự tạo `./data/3credit.db` và nạp dữ liệu từ `internal/store/seed.json`:
15 đối tác, 16 sản phẩm, 9 segment, lấy từ bản nghiên cứu ngày 05/11/2026.

| Biến môi trường | Ý nghĩa |
|---|---|
| `ADDR` | Địa chỉ lắng nghe, mặc định `:8080` |
| `BASE_URL` | URL công khai, ví dụ `https://3credit.vn` (bật cookie Secure) |
| `DATA_DIR` | Thư mục database và file tải lên, mặc định `./data` |
| `SESSION_SECRET` | Khóa ký CSRF và thông báo. **Bắt buộc khi production** |
| `DATA_KEY` | Khóa mã hóa dữ liệu lead (AES-GCM). **Bắt buộc khi production** |
| `ADMIN_USER`, `ADMIN_PASSWORD` | Tài khoản quản trị. Để trống thì tắt trang quản trị |
| `APP_ENV=production` | Từ chối khởi động nếu thiếu secret |

## Triển khai lên server bằng 1 lệnh (truy cập qua IP)

Cần một VPS Ubuntu/Debian (khuyến nghị 1 vCPU, 1 GB RAM trở lên). SSH vào server rồi chạy:

```bash
curl -fsSL https://raw.githubusercontent.com/lhoaibao1/3Credit/main/deploy.sh | sudo bash
```

Script tự làm các bước:
1. Cài Docker nếu chưa có.
2. Tải code về `/opt/3credit`.
3. Tạo `.env` với khóa bí mật và mật khẩu quản trị ngẫu nhiên.
4. Mở cổng 80 nếu firewall ufw đang bật.
5. Build và chạy container, tự khởi động lại khi server reboot.
6. In ra địa chỉ `http://<IP-server>`, `http://<IP-server>/admin` và mật khẩu quản trị.

- **Cập nhật** lên code mới nhất: chạy lại đúng lệnh trên. File `.env` và dữ liệu được giữ nguyên.
- **Cổng**: mặc định 80. Nếu cổng 80 đã bị chương trình khác dùng (Nginx, Apache, web khác), script tự chọn cổng trống (8080, 8081…) và in ra địa chỉ đúng. Muốn chỉ định cổng: `curl -fsSL …/deploy.sh | sudo PORT=8080 bash`.
- **Dữ liệu** (database, logo tải lên) nằm trong Docker volume `3credit_3credit-data`.
- **Mất `.env`** thì không giải mã được lead cũ. Hãy sao lưu file này.

> Truy cập qua IP là HTTP, chưa mã hóa. Dùng để chạy thử. Trước khi nhận thông tin khách thật, hãy gắn domain và bật HTTPS.

## Test

```bash
go test ./...
```

Các test đang có:
- Matching: chỉ điều kiện đã xác minh mới loại sản phẩm; hồ sơ nợ xấu không bao giờ “đáp ứng” khi chưa có chính sách xác minh; ưu tiên bị giới hạn; lọc theo nhu cầu.
- Segment: luật JSON, từ chối luật sai.
- Luồng đầy đủ: admin kích hoạt chiến dịch → khách bấm đăng ký → click ID (chống trùng) → postback sai chữ ký bị từ chối, đúng chữ ký ghi nhận một lần.
- Lead: không đồng ý thì không lưu; gửi lại không tạo trùng; số điện thoại mã hóa khi lưu; gửi sang CRM có `Idempotency-Key` và header xác thực.
- Bảo mật: trang quản trị cần đăng nhập; POST thiếu CSRF bị từ chối.

## Cấu trúc

```
cmd/server            khởi động, cấu hình, tắt máy an toàn
internal/model        kiểu dữ liệu chung
internal/db           SQLite + migration (internal/db/migrations/*.sql)
internal/store        đối tác, sản phẩm, chính sách có version, chiến dịch, segment, analytics, audit log, seed
internal/rules        ngôn ngữ luật JSON cho segment
internal/matching     Matching Engine: luật cứng → chấm điểm → xếp hạng, kèm lý do
internal/tracking     click (chống trùng 30 phút), URL đối tác, postback HMAC idempotent
internal/leads        đồng ý, lưu lead mã hóa, gửi CRM có retry/backoff
internal/server       HTTP: trang khách, trang quản trị, middleware bảo mật
web/templates         giao diện (html/template)
web/static            CSS, JS, logo 3Credit, logo đối tác
docs/research         nghiên cứu Giai đoạn 1
```

## Nguyên tắc nghiệp vụ trong code

- Chính sách có version, không sửa đè. Mỗi phiên bản bắt buộc có nguồn, ngày kiểm tra, người cập nhật, nội dung thay đổi.
- Giá trị mức `A`/`B` mới được dùng làm luật loại trừ. Giá trị mức `C` chỉ tạo mục “cần kiểm tra”.
- Hoa hồng không nằm trong điểm xếp hạng. Ưu tiên chiến dịch chỉ là điểm cộng 0–5 để phá thế hòa.
- Chiến dịch chỉ hiện nút đăng ký khi đã Active, có tracking URL và đang trong thời gian chạy.
- URL gửi sang đối tác chỉ chứa click ID. Không có tên, số điện thoại hay dữ liệu tín dụng.
- Lead chỉ được tạo khi khách đồng ý chia sẻ cho **một** đối tác được nêu tên.
- Conversion và hoa hồng chỉ đến từ postback có chữ ký hợp lệ. Hệ thống không tự tạo số liệu.

## Postback cho đối tác / affiliate network

```
GET|POST /api/postback/{network}?click_id=…&conversion_id=…&status=pending|approved|rejected&payout=…&sig=…
sig = hex(HMAC_SHA256(postback_secret, "click_id|conversion_id|status|payout"))
```

Postback secret của từng chiến dịch được đặt trong trang quản trị.

## Logo đối tác

`web/static/partners/` có sẵn logo lấy từ website chính thức của FE CREDIT, Shinhan Finance, VPBank, Moneyveo và F88. Logo của các đối tác còn lại tải lên trong **Quản trị → Đối tác**. Hệ thống nhận PNG, SVG, WebP tối đa 200 KB; SVG có script sẽ bị từ chối. Quyền dùng logo cần được ghi trong hợp đồng với từng đối tác.

## Việc tiếp theo

- Chuyển sang PostgreSQL khi lên production (schema đã viết bằng SQL tương thích).
- Đăng nhập quản trị nhiều người, phân quyền theo vai trò (hiện là một tài khoản Basic Auth).
- Adapter riêng cho từng affiliate network khi có tài liệu API.
- Rà soát pháp lý theo `docs/research/business-recommendations.md`.
