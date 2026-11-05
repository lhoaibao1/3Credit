# Customer Segments — Mô hình phân nhóm khách hàng

Ngày: **05/11/2026**

## 1. Nguyên tắc

1. **Segment được suy ra từ thuộc tính, không gán tay.** Mỗi segment là một tập luật (rule) trên các thuộc tính hồ sơ, được lưu trong DB, có version. Admin thêm, sửa segment mà không cần sửa code.
2. **Một khách có thể thuộc nhiều segment cùng lúc.** Ví dụ: "Nhân viên văn phòng" và "Cần tiền nhanh".
3. **Tình trạng tín dụng chỉ đến từ hai nguồn:**
   - Khách **tự khai báo** (`source = self_declared`).
   - Kết quả tra cứu từ tổ chức có quyền cung cấp thông tin tín dụng (CIC, PCB), **chỉ khi có đồng ý hợp lệ và hợp đồng dịch vụ** (`source = bureau`).

   **Không bao giờ** suy đoán nợ xấu từ hành vi, thiết bị, danh bạ hay dữ liệu khác.
4. **Thu thập tối thiểu.** Bước khám phá chỉ hỏi những gì luật matching cần. Không hỏi CCCD, số tài khoản hay số điện thoại trước khi khách chọn sản phẩm và đồng ý chia sẻ.

## 2. Thuộc tính hồ sơ (profile attributes)

| Mã | Thuộc tính | Kiểu | Bắt buộc ở bước khám phá | Ghi chú |
|---|---|---|---|---|
| `age` | Tuổi | số nguyên | Có | Hoặc năm sinh |
| `province` | Tỉnh/thành đang sống | mã tỉnh (danh mục 34 tỉnh/thành sau sáp nhập 2025, **cần xác nhận danh mục**) | Có | |
| `employment_type` | Nghề nghiệp | enum: `salaried_office`, `worker`, `civil_servant`, `self_employed`, `freelancer`, `online_seller`, `driver_gig`, `student`, `retired`, `unemployed` | Có | |
| `income_monthly` | Thu nhập/tháng | khoảng (VNĐ) | Có | Hỏi theo khoảng, không hỏi số chính xác |
| `income_channel` | Cách nhận thu nhập | enum: `bank_transfer`, `cash`, `mixed` | Có | |
| `income_proof` | Chứng minh được thu nhập | enum: `payslip_statement`, `social_insurance`, `business_license`, `none` | Không | Hỏi ở bước sau nếu cần |
| `need_type` | Nhu cầu | enum: `cash_loan`, `credit_card`, `installment`, `secured_vehicle`, `compare_only` | Có | |
| `amount_wanted` | Số tiền cần | số (VNĐ) | Có | |
| `term_wanted` | Kỳ hạn mong muốn | tháng | Có | |
| `urgency` | Mức gấp | enum: `today`, `this_week`, `flexible` | Không | |
| `credit_history` | Lịch sử tín dụng | enum: `none`, `good`, `past_late_resolved`, `group2`, `bad_settled`, `bad_unsettled` | Không (khách có thể bỏ qua) | Kèm `credit_source` |
| `bad_debt_group` | Nhóm nợ xấu | enum: 3, 4, 5 | Chỉ khi `credit_history` là nợ xấu | |
| `bad_debt_since` | Thời điểm phát sinh | tháng/năm | Không | |
| `existing_obligations` | Đang có khoản vay khác | số khoản + tổng trả/tháng (khoảng) | Không | Dùng để ước tính khả năng trả nợ (DTI) |
| `has_vehicle_registration` | Có giấy đăng ký xe đứng tên | enum: `motorbike`, `car`, `none` | Chỉ khi `need_type = secured_vehicle` | |
| `credit_source` | Nguồn dữ liệu tín dụng | enum: `self_declared`, `bureau` | Hệ thống tự gán | |

## 3. Danh mục segment khởi tạo

Luật viết theo dạng JSON-logic, lưu trong bảng `segments`. Cột "Tín hiệu matching" mô tả cách segment ảnh hưởng xếp hạng, không phải luật loại trừ.

| Mã | Tên | Luật (tóm tắt) | Tín hiệu matching |
|---|---|---|---|
| SEG01 | Nhân viên văn phòng | `employment_type ∈ {salaried_office, civil_servant}` AND `income_channel = bank_transfer` | Ưu tiên vay tín chấp theo lương của ngân hàng |
| SEG02 | Công nhân, người lao động | `employment_type = worker` | Lương tiền mặt → loại các sản phẩm yêu cầu lương chuyển khoản (khi đã xác minh) |
| SEG03 | Tự kinh doanh, tự do | `employment_type ∈ {self_employed, freelancer, online_seller, driver_gig}` | Ưu tiên sản phẩm chấp nhận thu nhập thay thế, sao kê, hóa đơn |
| SEG04 | Chưa có lịch sử tín dụng | `credit_history = none` | Ưu tiên sản phẩm hạn mức nhỏ, thẻ tín dụng nhập môn |
| SEG05 | Lịch sử tín dụng tốt | `credit_history = good` | Mở toàn bộ sản phẩm ngân hàng |
| SEG06 | Từng quá hạn | `credit_history ∈ {past_late_resolved, group2}` | Tách hai trường hợp: đã xử lý xong và đang ở nhóm 2 |
| SEG07a | Nợ xấu đã tất toán | `credit_history = bad_settled` | Chỉ hiện sản phẩm có chính sách **đã xác minh** nhận trường hợp này |
| SEG07b | Nợ xấu chưa tất toán | `credit_history = bad_unsettled` | Như trên; thêm nội dung hướng dẫn xử lý nợ |
| SEG07-G3/G4/G5 | Theo nhóm nợ | `bad_debt_group = 3/4/5` | Segment con của SEG07, để đối tác khai báo điều kiện theo nhóm |
| SEG08 | Cần tiền nhanh | `urgency = today` | Ưu tiên giải ngân nhanh; **không** được nới điều kiện |
| SEG09 | Có xe làm tài sản | `has_vehicle_registration ≠ none` | Mở sản phẩm cầm cố đăng ký xe |
| SEG10 | Gánh nặng nợ cao | `existing_obligations.monthly / income_monthly > 0.5` | Cảnh báo khả năng trả nợ; hạ thứ hạng các khoản vay lớn |

SEG10 là gợi ý thêm để bảo vệ người vay, không có trong brief.

## 4. Ma trận segment × đối tác (hiện trạng)

Ký hiệu:
- ✔ = chính sách đã xác minh là nhận
- ✖ = đã xác minh là loại trừ
- ~ = có dấu hiệu từ nguồn C, chưa xác minh
- ? = chưa có dữ liệu

| | SEG01 | SEG02 | SEG03 | SEG04 | SEG06 | SEG07 | SEG09 |
|---|---|---|---|---|---|---|---|
| Shinhan Finance | ~ | ? | ~ (thu nhập thay thế) | ? | ? | ? | — |
| VIB | ~ (lương CK) | ~ ✖ nếu lương tiền mặt | ? | ? | ? | ? | — |
| FE CREDIT | ? | ? | ? | ? | ? | ? | — |
| F88 | ? | ? | ~ (không cần CM thu nhập) | ? | ? | ~ ("không xét nợ xấu", C) | ✔ (sản phẩm cầm cố xe) |
| Các đối tác còn lại | ? | ? | ? | ? | ? | ? | — |

→ Ma trận gần như trống. **Việc đầu tiên khi làm việc với từng đối tác** là gửi bảng hỏi chính sách (template ở `business-recommendations.md`) để điền ma trận này.

## 5. Quy tắc hiển thị cho SEG07 (nợ xấu)

1. Sản phẩm có luật loại trừ đã xác minh cho nhóm nợ của khách → `Not eligible`, không hiển thị trong danh sách phù hợp.
2. Sản phẩm chưa có dữ liệu CIC → `Unverified`, chỉ hiện ở mục "cần kiểm tra thêm", kèm nhãn rõ ràng.
3. Không có sản phẩm `Eligible` nào → hiện thông báo trung thực: chưa tìm thấy sản phẩm phù hợp. Kèm nội dung hướng dẫn: cách tra cứu CIC, cách xử lý nợ, cảnh báo tín dụng đen.
4. Cấm các câu như "100% duyệt", "nhận mọi nợ xấu", "không cần CIC".
