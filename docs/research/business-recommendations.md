# Business Recommendations — Khuyến nghị nghiệp vụ (kết thúc Giai đoạn 1)

Ngày: **05/11/2026**

## 1. Kết luận chính

1. **Dữ liệu công khai không đủ để chạy matching.** Với 15 đối tác, mới có 4 đối tác có vài trường được xác minh từ website chính thức (Shinhan Finance, FE CREDIT, F88, Moneyveo). **Không đối tác nào** công bố điều kiện CIC/nợ xấu đủ tin cậy. Nguồn dữ liệu chính sách thật phải là **tài liệu đối tác gửi qua hợp đồng affiliate**, kèm website chính thức.
2. **Nhóm B có rủi ro pháp lý cao hơn nhóm A.** MoneyCAT, Moneyveo, Jeff, Vay VND không xuất hiện trong danh sách hội viên VNBA đã tra. Nếu không phải tổ chức tín dụng thì lãi suất bị giới hạn 20%/năm (BLDS 2015, Điều 468), và tổng chi phí (lãi + "phí tư vấn") là điểm dễ bị xem là lách trần. Nguồn tham khảo về MoneyCAT ghi lãi tối đa 24%/năm, cao hơn trần này.
3. **Mô hình hợp lý:** 3Credit là **bên giới thiệu (referral/affiliate)**, không phải bên cho vay, không thẩm định, không tra CIC (trừ khi sau này ký hợp đồng với CIC/PCB). Mọi thông điệp phải nhất quán với vai trò này.

## 2. Điều kiện để một đối tác được `Active` trên hệ thống

Cấu hình thành checklist bắt buộc trong Partner Management:

- [ ] Pháp nhân + mã số doanh nghiệp
- [ ] Giấy phép hoạt động (TCTD: giấy phép NHNN; cầm đồ: giấy chứng nhận đủ điều kiện ANTT…), có file đính kèm
- [ ] Hợp đồng affiliate/giới thiệu còn hiệu lực
- [ ] Bảng chính sách sản phẩm theo template mục 4, có chữ ký hoặc email xác nhận
- [ ] APR tối đa **đã gồm toàn bộ phí**, kỳ hạn tối thiểu–tối đa, ví dụ tổng chi phí
- [ ] Kênh thu hồi nợ và cam kết không đòi nợ bằng cách liên hệ người thân, bêu riếu
- [ ] Tracking URL, SubID, postback do đối tác hoặc network cấp. **3Credit không tự tạo**

## 3. Nguyên tắc xếp hạng

```
final_score = fit_score (0–100)
            × data_confidence (0.5–1.0)
            × freshness_factor (0.7–1.0, theo ngày xác minh)
            + admin_priority_boost (giới hạn tối đa +5)
```

- Hoa hồng **không** nằm trong công thức hiển thị cho khách. Chỉ dùng để phá thế hòa khi hai sản phẩm bằng điểm, và phải được ghi log.
- Sản phẩm `Unverified` không bao giờ đứng trên sản phẩm `Eligible`.
- Mỗi kết quả trả về kèm `reasons[]` (vì sao phù hợp) và `gaps[]` (còn thiếu gì).

## 4. Template bảng hỏi chính sách gửi đối tác

| Trường | Đối tác điền |
|---|---|
| Tên sản phẩm, loại | |
| Tuổi tối thiểu, tối đa (tính tại thời điểm vay hay khi kết thúc khoản vay?) | |
| Thu nhập tối thiểu; hình thức chứng minh chấp nhận | |
| Nghề nghiệp chấp nhận / loại trừ | |
| Tỉnh thành hỗ trợ | |
| Điều kiện CIC: nhận nhóm 1/2? nhóm 3–5? nợ xấu đã tất toán bao lâu? | |
| Số khoản vay đang có tối đa tại TCTD khác | |
| Hạn mức tối thiểu, tối đa | |
| Lãi suất (cách tính: dư nợ giảm dần / phẳng / theo ngày), APR tối đa gồm phí | |
| Các loại phí (hồ sơ, bảo hiểm, tư vấn, trả trước hạn, quá hạn) | |
| Kỳ hạn tối thiểu, tối đa | |
| Hồ sơ cần | |
| Quy trình (online / tại điểm / kết hợp), thời gian xử lý | |
| Điều kiện loại trừ khác | |
| Quy định quảng cáo của đối tác (từ cấm, nội dung bắt buộc) | |
| Lead cap ngày / tổng; quy định chống gian lận | |
| Mô hình hoa hồng (CPL / CPA / CPS / RevShare), sự kiện tính hoa hồng, thời gian đối soát | |
| Postback: URL format, tham số, IP whitelist, chữ ký | |

## 5. Tuân thủ — các văn bản phải rà soát (Giai đoạn 2 cần luật sư xác nhận)

| Lĩnh vực | Văn bản | Ảnh hưởng tới thiết kế |
|---|---|---|
| Dữ liệu cá nhân | Luật Bảo vệ dữ liệu cá nhân 2025 (hiệu lực 01/01/2026), Nghị định 356/2025/NĐ-CP (thay NĐ 13/2023) | Đồng ý theo từng mục đích và từng bên nhận; quyền rút đồng ý, xóa dữ liệu; thông báo sự cố lộ dữ liệu nhạy cảm trong 72 giờ; dữ liệu tài chính, tín dụng là **dữ liệu nhạy cảm** |
| Bảo vệ người tiêu dùng | Luật Bảo vệ quyền lợi người tiêu dùng 2023 | Thông tin trung thực, không gây nhầm lẫn; điều kiện giao dịch chung |
| Cho vay dân sự | Bộ luật Dân sự 2015, Điều 468 (trần 20%/năm); BLHS Điều 201 | Điều kiện nhận đối tác nhóm B |
| Cho vay tiêu dùng của công ty tài chính | Quy định của NHNN về cho vay tiêu dùng (cần luật sư xác định văn bản hiện hành) | Nội dung bắt buộc công bố |
| Quảng cáo | Luật Quảng cáo và quy định chuyên ngành; chính sách quảng cáo tài chính của Google, Meta, TikTok | Không cam kết duyệt; công bố APR, kỳ hạn, ví dụ chi phí |
| Thông tin tín dụng | Quy định về hoạt động thông tin tín dụng (CIC, công ty thông tin tín dụng) | 3Credit **không** tự tra cứu CIC nếu chưa có hợp đồng và đồng ý |

## 6. Đề xuất kiến trúc sơ bộ (để thảo luận ở Giai đoạn 2)

Hiện không có repository nào, chỉ có thư mục `brand/` (logo) và bản mockup HTML. Đề xuất mặc định, sẽ chốt ở Giai đoạn 2:

- **Web khách hàng + Admin:** Next.js (App Router, TypeScript), render phía server cho SEO
- **API, domain:** NestJS hoặc Next.js route handlers, chia module theo brief (Partner, Catalog, Policy Registry, Segmentation, Matching, Campaign, Tracking, Lead Routing, Conversion, Commission, Analytics)
- **CSDL:** PostgreSQL. Luật điều kiện lưu dạng JSONB có version (bảng `policy_versions`). Mã hóa cột dữ liệu nhạy cảm
- **Hàng đợi:** Redis + BullMQ, cho lead routing, postback, retry
- **Tracking:** Endpoint redirect `/go/:clickId`, chỉ truyền `click_id` và `sub_id`, **không truyền dữ liệu cá nhân** trong URL
- **Hạ tầng:** Đặt máy chủ tại Việt Nam hoặc đánh giá yêu cầu chuyển dữ liệu ra nước ngoài theo Luật BVDLCN

## 7. Câu hỏi cần bạn trả lời trước Giai đoạn 2

1. Đã ký hợp đồng affiliate với đối tác nào? Qua **affiliate network** nào (ví dụ AccessTrade, MasOffer, Dinos…) hay ký trực tiếp?
2. "EVO" là **TPBank EVO** (thẻ) hay **EVO Money** (vay)?
3. Vay VND: bạn gửi giúp bản giấy phép và tên pháp nhân để lưu vào hồ sơ đối tác.
4. 3Credit có pháp nhân chưa? Tên pháp nhân sẽ đứng tên là **bên kiểm soát dữ liệu cá nhân**.
5. "Hệ thống nội bộ" nhận lead là gì: CRM nào, có API không, hay đội telesale?
6. Chọn stack: đồng ý với đề xuất mục 6 hay đã có hạ tầng hoặc đội kỹ thuật riêng?
7. Có cần tài khoản người dùng (đăng nhập, lịch sử đăng ký) ngay từ bản đầu không?
