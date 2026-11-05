# 3Credit Brand Guidelines

**3Credit · Find Your Financial Fit**
3Credit thuộc hệ sinh thái 3RDHub Vietnam. Logo kế thừa trực tiếp mascot và ngôn ngữ chữ của 3RDHub.

## 0. Phiên bản 2 (05/11/2026): Thẻ-robot

Biểu tượng được làm lại theo yêu cầu “giữ robot, thêm chất tín dụng”. Màn hình robot giờ có tỷ lệ của một **thẻ thanh toán** (~1,6 : 1) và có **dải từ** ngang phía trên. Khuôn mặt robot của 3RDHub giữ nguyên: mắt trái là viên dọc màu trắng, mắt phải là **dấu tick cam**.

Đã thử 3 phương án (`brand/preview/concepts-v2.png`):
- **D · Thẻ-robot (chọn):** đọc ra “thẻ tín dụng” ngay, vẫn rõ ở 16px.
- **E · Robot cầm thẻ:** chiếc thẻ nhỏ ở góc bị rối khi thu nhỏ.
- **F · Ăng-ten đo điểm:** vòng cung mất hẳn ở cỡ favicon.

Các mục bên dưới (màu, chữ, khoảng cách, cấm kỵ) vẫn áp dụng. Phần “Ba concept” ở mục 2 là lịch sử của phiên bản 1.

## 1. Phân tích logo gốc 3RDHub Vietnam

| Thành phần | 3RDHub Vietnam (đo từ favicon.svg và icon-512.png) |
|---|---|
| Mascot | Màn hình bo góc 42×30, góc bo 13, nét trắng 4.4 trên lưới 64. Mắt trái là viên dọc màu trắng 5.6×13. Mắt phải là cung cam hình ^, dây cung 11, bán kính 6.6 |
| Chữ | Nunito Black (900), bo tròn, khoảng cách chữ khoảng −0.025em |
| Màu | Navy `#0B2A6F`, cam `#FF6A2B`, trắng `#F4F6FF` |
| Bố cục | Phần trắng đi trước, phần cam đi sau ("3RDHub" trắng, "Vietnam" cam) |

## 2. Ba concept và cách chọn

| Tiêu chí (1–5) | A · Giữ nguyên | B · Score Eye | C · Fit Check ✅ |
|---|---|---|---|
| Dễ nhận ra | 3 | 2 | 5 |
| Kế thừa 3RDHub | 5 | 4 | 4 |
| Chuyên nghiệp | 4 | 3 | 5 |
| Dùng trên web/app | 3 | 3 | 5 |
| Hiển thị ở cỡ nhỏ | 5 | 2 | 4 |

- **A:** Robot giữ nguyên 100%. Favicon trùng với 3RDHub nên người dùng không phân biệt được hai trang.
- **B:** Cung cam thêm một chấm, ý là đồng hồ điểm. Bị loại vì nhìn giống biểu tượng **wifi**.
- **C (chọn):** Mắt cam đổi thành dấu tick, như robot nháy mắt xác nhận "đã tìm được lựa chọn phù hợp". Ý này khớp với tagline *Find Your Financial Fit*. Hình dáng robot, nét, màu và vị trí mắt giữ nguyên lưới của 3RDHub.

Xem bản so sánh tại `brand/preview/concepts.png`.

## 3. Danh sách file

| File | Dùng khi |
|---|---|
| `3credit-primary.svg` | Logo chính kèm nền navy: slide, ảnh chia sẻ mạng xã hội, tài liệu |
| `3credit-horizontal.svg` | Header web và app trên nền tối. Nền trong suốt, không tagline |
| `3credit-horizontal-light.svg` | Header trên nền sáng. Nền trong suốt, không tagline |
| `3credit-stacked.svg` | Màn hình chờ, poster, trang đăng nhập (nền tối) |
| `3credit-icon.svg` | Icon app, ảnh đại diện mạng xã hội |
| `3credit-favicon.svg` | Favicon trình duyệt (nét dày hơn 15%, khung chặt hơn) |
| `3credit-logo-dark.svg` | Logo kèm tagline trên nền navy hoặc nền tối |
| `3credit-logo-light.svg` | Logo kèm tagline trên nền trắng hoặc nền sáng |
| `3credit-logo-mono.svg` | Đơn sắc, dùng `currentColor`, đổi màu bằng CSS `color` |

Tất cả chữ đã được chuyển thành đường vector (outline) nên không phụ thuộc font. Mỗi file có `viewBox` chuẩn, không có width/height cố định, nhúng thẳng vào web được.

```html
<link rel="icon" href="/brand/3credit-favicon.svg" type="image/svg+xml">
<img src="/brand/3credit-horizontal.svg" alt="3Credit" height="36">
```

## 4. Màu sắc

| Tên | HEX | Vai trò |
|---|---|---|
| Navy | `#101B4D` | Nền thương hiệu, chữ "3" trên nền sáng |
| Orange | `#FF642E` | Chữ "Credit", dấu tick, nút CTA chính. Là màu nhấn duy nhất |
| White | `#FFFFFF` | Mascot và chữ "3" trên nền tối |
| Mist | `#AEB8DA` | Tagline trên nền tối |
| Slate | `#5A648A` | Tagline trên nền sáng |
| Blue (phụ) | `#3D7BFF` | Chỉ dùng cho link và biểu đồ trong sản phẩm, không dùng trong logo |

Màu cam không được đổi sang xanh lá hay xanh dương. Không dùng gradient cho logo.

## 5. Chữ

- **Wordmark:** Nunito Black 900, khoảng cách chữ −0.02em. "3" trắng (hoặc navy trên nền sáng), "Credit" cam.
- **Tagline:** Nunito ExtraBold 800, viết HOA, khoảng cách chữ +0.14em, dài đúng bằng wordmark.
- **Giao diện sản phẩm:** Nunito 600–900 cho tiêu đề và nội dung.

## 6. Tỷ lệ và khoảng trống

Gọi **H** là chiều cao biểu tượng robot (tính cả nét).

- Chiều cao chữ hoa của wordmark bằng 0.70 H. Khoảng cách từ robot đến chữ bằng 0.30 H.
- Khoảng trống quanh logo tối thiểu 0.25 H. Các file SVG đã chừa sẵn khoảng này.
- Bản stacked: khoảng cách robot → wordmark = 0.32 H; wordmark → tagline = 0.32 lần chiều cao chữ hoa.

## 7. Kích thước tối thiểu

| Bản | Tối thiểu |
|---|---|
| Có tagline | Rộng 180px (in ấn: 40mm) |
| Ngang không tagline | Rộng 96px, cao 24px |
| Icon / favicon | 16px (dùng `3credit-favicon.svg`) |

## 8. Không được làm

- Kéo méo, xoay, tách robot khỏi wordmark trong bản ngang
- Đổi màu cam, thêm gradient, đổ bóng, phát sáng, hiệu ứng 3D
- Đặt bản trắng lên nền sáng, hoặc đặt logo lên ảnh rối mà không có lớp nền
- Thay mắt tick bằng biểu tượng khác (đồng tiền, ví, mũi tên…)
- Viết sai tên: chỉ dùng **3Credit**, không viết "3 Credit", "3CREDIT" hay "3credit"

## 9. Kiểm tra trước khi bàn giao

Toàn bộ các bước dưới đây đã được dựng thử tại `brand/preview/brand-preview.html` (ảnh chụp: `brand-preview.png`):

- Đặt cạnh logo 3RDHub
- Từng biến thể trên nền navy, nền trắng, nền cam (mono)
- Favicon ở 64, 32 và 16px, có thử trên tab trình duyệt
- Header web nền navy và nền trắng
- Màn hình mobile, màn hình chờ, icon app
