package server

import (
	"net/http"
	"net/url"
)

// productPage describes one product's own registration form. A visitor who clicked a
// product goes straight to its form; only visitors who have not chosen yet get the
// step-by-step "what do you need?" flow on the home page.
type productPage struct {
	Need, Slug, Title, Lede, Art, Photo, Submit, FormTitle, FormNote string
	Points                                                           []string
}

var productPages = []productPage{
	{Need: "cash_loan", Slug: "vay-tien-mat", FormTitle: "Đăng ký vay tiền mặt", FormNote: "Khoảng 1 phút. Hỏi về khoản vay và thu nhập của bạn.", Photo: "vay-tien-mat", Title: "Vay tiền mặt", Art: "cash",
		Lede:   "Khai số tiền cần vay và vài thông tin cơ bản, 3Credit đối chiếu với điều kiện các công ty tài chính, ngân hàng công bố.",
		Submit: "Xem khoản vay phù hợp",
		Points: []string{"So sánh nhiều đối tác cùng lúc", "Thấy rõ điều kiện nào đã khớp", "Không cần CCCD để xem đề xuất"}},
	{Need: "credit_card", Slug: "the-tin-dung", FormTitle: "Đăng ký mở thẻ tín dụng", FormNote: "Khoảng 1 phút. Hỏi về nhu cầu dùng thẻ và thu nhập để ước hạn mức.", Photo: "the-tin-dung", Title: "Mở thẻ tín dụng", Art: "card",
		Lede:   "Chọn hạn mức mong muốn và khai thu nhập, 3Credit gợi ý những thẻ bạn có khả năng mở được.",
		Submit: "Xem thẻ phù hợp",
		Points: []string{"Đối chiếu theo thu nhập của bạn", "So sánh hạn mức và phí", "Không ảnh hưởng điểm tín dụng"}},
	{Need: "secured", Slug: "vay-dang-ky-xe", FormTitle: "Đăng ký vay bằng đăng ký xe", FormNote: "Khoảng 1 phút. Hỏi về chiếc xe trước, rồi mới tới khoản vay.", Title: "Vay bằng đăng ký xe", Art: "bike",
		Lede:   "Dùng giấy đăng ký xe máy hoặc ô tô đứng tên bạn làm tài sản bảo đảm. Khai loại xe và số tiền cần vay để xem các gói phù hợp.",
		Submit: "Xem gói vay phù hợp",
		Points: []string{"Dành cho người có xe máy, ô tô", "Hạn mức theo giá trị xe", "Chỉ đối tác có pháp nhân rõ ràng"}},
	{Need: "compare", Slug: "so-sanh-tat-ca", FormTitle: "Khai hồ sơ để so sánh", FormNote: "Khai một lần cho mọi loại sản phẩm: vay tiền, thẻ, vay bằng xe.", Photo: "so-sanh", Title: "So sánh tất cả sản phẩm", Art: "compare",
		Lede:   "Chưa rõ nên chọn vay hay mở thẻ? Khai hồ sơ một lần, 3Credit xếp mọi sản phẩm theo mức phù hợp với bạn.",
		Submit: "So sánh ngay",
		Points: []string{"Mọi loại sản phẩm trong một lần", "Xếp theo mức phù hợp", "Chọn tối đa 3 sản phẩm đặt cạnh nhau"}},
}

// xfield is a question only one product asks (loan purpose, card usage, vehicle details).
// The answers travel with the profile link and are shown back in the summary.
type xfield struct {
	Key, Label string
	Opts       []opt
	Val        string
}

var productExtras = map[string][]xfield{
	"cash_loan": {
		{Key: "purpose", Label: "Bạn vay để làm gì?", Opts: []opt{{"tieu_dung", "Tiêu dùng cá nhân"}, {"sua_nha", "Sửa nhà"}, {"mua_sam", "Mua sắm thiết bị"}, {"y_te", "Y tế, học phí"}, {"kinh_doanh", "Kinh doanh nhỏ"}, {"khac", "Việc khác"}}},
	},
	"credit_card": {
		{Key: "card_use", Label: "Bạn dùng thẻ chủ yếu để", Opts: []opt{{"mua_sam", "Mua sắm hằng ngày"}, {"tra_gop", "Trả góp 0%"}, {"hoan_tien", "Hoàn tiền"}, {"du_lich", "Du lịch"}, {"rut_tien", "Rút tiền mặt"}}},
		{Key: "has_card", Label: "Bạn đang có thẻ tín dụng chưa?", Opts: []opt{{"none", "Chưa có"}, {"one", "Đang có 1 thẻ"}, {"many", "Từ 2 thẻ trở lên"}}},
	},
	"secured": {
		{Key: "veh_year", Label: "Xe đăng ký năm nào?", Opts: []opt{{"y2022", "Từ 2022 đến nay"}, {"y2018", "2018 – 2021"}, {"y2012", "2012 – 2017"}, {"old", "Trước 2012"}}},
		{Key: "veh_owner", Label: "Giấy đăng ký đứng tên", Opts: []opt{{"self", "Chính chủ (tên bạn)"}, {"other", "Chưa sang tên"}}},
		{Key: "veh_loan", Label: "Xe có đang trả góp không?", Opts: []opt{{"clear", "Không, đã trả hết"}, {"paying", "Còn đang trả góp"}}},
	},
}

// extrasFor reads a product's own answers from the query; unknown values fall back to the first option.
func extrasFor(need string, qv url.Values) (map[string]xfield, url.Values, []string) {
	fields, keep, labels := map[string]xfield{}, url.Values{}, []string{}
	for _, f := range productExtras[need] {
		f.Val = pick(qv.Get(f.Key), f.Opts, f.Opts[0].V)
		fields[f.Key] = f
		keep.Set(f.Key, f.Val)
		for _, o := range f.Opts {
			if o.V == f.Val {
				labels = append(labels, o.L)
			}
		}
	}
	return fields, keep, labels
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
		xf, xkeep, xlabels := extrasFor(pp.Need, qv)
		full := profileQuery(p)
		for k, v := range xkeep {
			full[k] = v
		}
		data["XF"], data["XLabels"], data["Query"] = xf, xlabels, q(full)
		data["Back"], data["Source"] = "/"+pp.Slug+"?"+full.Encode(), pp.Slug
		if done {
			data["Back"] = data["Back"].(string) + "&xem=1"
		}
		data["Regions"], data["Jobs"], data["Incomes"] = regions, jobs, incomes
		data["Channels"], data["Credits"], data["Urgencies"] = channels, credits, urgencies
		data["Amounts"], data["Terms"] = []int{10, 20, 30, 50, 70, 100, 200, 300}, []int{6, 12, 18, 24, 36, 48, 60}
		if pp.Need == "credit_card" {
			data["Amounts"] = []int{10, 20, 30, 50, 100, 200}
		}
		data["Vehicles"] = vehicles
		if pp.Need == "secured" {
			data["Vehicles"] = vehicles[1:] // this form only lists vehicles you can pledge
		}
		s.render(w, "product.html", data)
	}
}

// chooser asks "what do you need?" for visitors who have not picked a product yet.
func (s *Server) chooser(w http.ResponseWriter, r *http.Request) {
	data := s.base(r, "Bạn cần gì? – 3Credit")
	data["Pages"], data["Source"] = productPages, "tim-san-pham"
	s.render(w, "chooser.html", data)
}
