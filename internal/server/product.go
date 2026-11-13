package server

import (
	"net/http"
	"net/url"
)

// productPage describes one product's own registration form. A visitor who clicked a
// product goes straight to its form; only visitors who have not chosen yet get the
// step-by-step "what do you need?" flow on the home page.
type productPage struct {
	Need, Slug, Title, Lede, Art, Submit string
	Points                               []string
}

var productPages = []productPage{
	{Need: "cash_loan", Slug: "vay-tien-mat", Title: "Vay tiền mặt", Art: "cash",
		Lede:   "Khai số tiền cần vay và vài thông tin cơ bản, 3Credit đối chiếu với điều kiện các công ty tài chính, ngân hàng công bố.",
		Submit: "Xem khoản vay phù hợp",
		Points: []string{"So sánh nhiều đối tác cùng lúc", "Thấy rõ điều kiện nào đã khớp", "Không cần CCCD để xem đề xuất"}},
	{Need: "credit_card", Slug: "the-tin-dung", Title: "Mở thẻ tín dụng", Art: "card",
		Lede:   "Chọn hạn mức mong muốn và khai thu nhập, 3Credit gợi ý những thẻ bạn có khả năng mở được.",
		Submit: "Xem thẻ phù hợp",
		Points: []string{"Đối chiếu theo thu nhập của bạn", "So sánh hạn mức và phí", "Không ảnh hưởng điểm tín dụng"}},
	{Need: "secured", Slug: "vay-dang-ky-xe", Title: "Vay bằng đăng ký xe", Art: "bike",
		Lede:   "Dùng giấy đăng ký xe máy hoặc ô tô đứng tên bạn làm tài sản bảo đảm. Khai loại xe và số tiền cần vay để xem các gói phù hợp.",
		Submit: "Xem gói vay phù hợp",
		Points: []string{"Dành cho người có xe máy, ô tô", "Hạn mức theo giá trị xe", "Chỉ đối tác có pháp nhân rõ ràng"}},
	{Need: "compare", Slug: "so-sanh-tat-ca", Title: "So sánh tất cả sản phẩm", Art: "compare",
		Lede:   "Chưa rõ nên chọn vay hay mở thẻ? Khai hồ sơ một lần, 3Credit xếp mọi sản phẩm theo mức phù hợp với bạn.",
		Submit: "So sánh ngay",
		Points: []string{"Mọi loại sản phẩm trong một lần", "Xếp theo mức phù hợp", "Chọn tối đa 3 sản phẩm đặt cạnh nhau"}},
}

func productBySlug(slug string) (productPage, bool) {
	for _, p := range productPages {
		if p.Slug == slug {
			return p, true
		}
	}
	return productPage{}, false
}

// ProductPath is the form page of a need ("" for an unknown need).
func ProductPath(need string) string {
	for _, p := range productPages {
		if p.Need == need {
			return "/" + p.Slug
		}
	}
	return ""
}

func (s *Server) productForm(pp productPage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qv := r.URL.Query()
		done := qv.Get("xem") == "1"
		pv := url.Values{}
		for k, v := range qv {
			pv[k] = v
		}
		pv.Set("need", pp.Need) // the page decides the product, never the query
		if pp.Need == "secured" && pv.Get("vehicle") == "" {
			pv.Set("vehicle", "motorbike")
		}
		p := ParseProfile(pv)
		data, err := s.results(r, p, done)
		if err != nil {
			s.fail(w, err)
			return
		}
		for k, v := range s.base(r, pp.Title+" – 3Credit") {
			data[k] = v
		}
		data["PP"], data["Done"] = pp, done
		data["Back"], data["Source"] = "/"+pp.Slug+"?"+profileQuery(p).Encode(), pp.Slug
		if done {
			data["Back"] = data["Back"].(string) + "&xem=1"
		}
		data["Regions"], data["Jobs"], data["Incomes"] = regions, jobs, incomes
		data["Channels"], data["Credits"], data["Urgencies"] = channels, credits, urgencies
		data["Vehicles"] = vehicles
		if pp.Need == "secured" {
			data["Vehicles"] = vehicles[1:] // this form only lists vehicles you can pledge
		}
		s.render(w, "product.html", data)
	}
}
