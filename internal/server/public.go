package server

import (
	"database/sql"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/matching"
	"github.com/threecredit/platform/internal/model"
	"github.com/threecredit/platform/internal/rules"
	"github.com/threecredit/platform/internal/store"
	"github.com/threecredit/platform/internal/tracking"
)

type opt struct{ V, L string }

var (
	needs     = []opt{{"cash_loan", "Vay tiền"}, {"credit_card", "Thẻ tín dụng"}, {"secured", "Vay bằng xe"}, {"compare", "Tất cả"}}
	regions   = []opt{{"hn", "Hà Nội"}, {"hcm", "TP. Hồ Chí Minh"}, {"dn", "Đà Nẵng"}, {"hp", "Hải Phòng"}, {"ct", "Cần Thơ"}, {"other", "Tỉnh/thành khác"}}
	jobs      = []opt{{"salaried_office", "Nhân viên văn phòng"}, {"civil_servant", "Công chức, viên chức, y tế, giáo dục"}, {"worker", "Công nhân, lao động phổ thông"}, {"self_employed", "Chủ hộ kinh doanh"}, {"online_seller", "Kinh doanh online"}, {"freelancer", "Làm tự do (freelancer)"}, {"driver_gig", "Tài xế công nghệ"}, {"student", "Sinh viên"}, {"retired", "Nghỉ hưu"}}
	incomes   = []opt{{"2", "Dưới 3 triệu"}, {"4", "3–5 triệu"}, {"6", "5–7 triệu"}, {"9", "7–10 triệu"}, {"12", "10–15 triệu"}, {"20", "15–25 triệu"}, {"30", "Trên 25 triệu"}}
	channels  = []opt{{"bank_transfer", "Chuyển khoản"}, {"cash", "Tiền mặt"}, {"mixed", "Cả hai"}}
	credits   = []opt{{"", "Không muốn trả lời"}, {"good", "Tốt, chưa từng trễ hạn"}, {"none", "Chưa từng vay"}, {"past_late_resolved", "Từng trễ hạn, đã xử lý xong"}, {"group2", "Đang nợ nhóm 2"}, {"bad_settled", "Từng nợ xấu, đã tất toán"}, {"bad_unsettled", "Đang có nợ xấu chưa tất toán"}}
	urgencies = []opt{{"flexible", "Không gấp"}, {"this_week", "Trong tuần"}, {"today", "Cần ngay hôm nay"}}
	vehicles  = []opt{{"none", "Không có"}, {"motorbike", "Xe máy"}, {"car", "Ô tô"}}
)

func pick(v string, opts []opt, def string) string {
	for _, o := range opts {
		if o.V == v {
			return v
		}
	}
	return def
}

func clampInt(s string, lo, hi, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// ParseProfile reads a profile from query values with safe defaults.
func ParseProfile(v url.Values) model.Profile {
	inc, _ := strconv.ParseFloat(pick(v.Get("income"), incomes, "12"), 64)
	credit := "good"
	if _, ok := v["credit"]; ok {
		credit = pick(v.Get("credit"), credits, "")
	}
	return model.Profile{
		Need:    pick(v.Get("need"), needs, "cash_loan"),
		Amount:  float64(clampInt(v.Get("amount"), 1, 500, 50)) * 1e6,
		Term:    clampInt(v.Get("term"), 1, 60, 24),
		Age:     clampInt(v.Get("age"), 16, 90, 28),
		Region:  pick(v.Get("region"), regions, "hcm"),
		Job:     pick(v.Get("job"), jobs, "salaried_office"),
		Income:  inc,
		Channel: pick(v.Get("channel"), channels, "bank_transfer"),
		Credit:  credit,
		Urgency: pick(v.Get("urgency"), urgencies, "flexible"),
		Vehicle: pick(v.Get("vehicle"), vehicles, "none"),
	}
}

func profileQuery(p model.Profile) url.Values {
	v := url.Values{}
	v.Set("need", p.Need)
	v.Set("amount", strconv.Itoa(int(p.Amount/1e6)))
	v.Set("term", strconv.Itoa(p.Term))
	v.Set("age", strconv.Itoa(p.Age))
	v.Set("region", p.Region)
	v.Set("job", p.Job)
	v.Set("income", strconv.FormatFloat(p.Income, 'f', -1, 64))
	v.Set("channel", p.Channel)
	v.Set("credit", p.Credit)
	v.Set("urgency", p.Urgency)
	v.Set("vehicle", p.Vehicle)
	return v
}

type group struct {
	Key, Title string
	Items      []matching.Result
}

var creditText = map[string]string{"group2": "nợ nhóm 2", "bad_settled": "nợ xấu đã tất toán", "bad_unsettled": "nợ xấu chưa tất toán"}

// results runs the matching engine and builds the template data.
func (s *Server) results(r *http.Request, p model.Profile, logIt bool) (map[string]any, error) {
	offers, err := s.st.Offers()
	if err != nil {
		return nil, err
	}
	segAll, err := s.st.Segments()
	if err != nil {
		return nil, err
	}
	segs := rules.Segments(segAll, p)
	segIDs := make([]string, len(segs))
	for i, g := range segs {
		segIDs[i] = g.ID
	}
	now := s.now()
	// customers see draft/rejected/expired campaigns never; paused never.
	var visible []model.Offer
	for _, o := range offers {
		st := o.Campaign.Status
		if st != model.StatusActive && st != model.StatusPending {
			continue
		}
		if o.Campaign.EndAt != nil && now.After(*o.Campaign.EndAt) {
			continue
		}
		if !targets(o.Campaign.TargetSegments, segIDs) {
			continue
		}
		visible = append(visible, o)
	}
	rs := matching.Match(visible, p, func(c model.Campaign) bool { return c.Live(now) })
	cnt := matching.Counts(rs)
	counts := map[string]int{}
	for k, v := range cnt {
		counts[string(k)] = v
	}
	gs := []group{{Key: "eligible", Title: "Phù hợp nhất"}, {Key: "potential", Title: "Cần kiểm tra thêm"}, {Key: "unverified", Title: "Chưa đủ dữ liệu"}}
	var nots []matching.Result
	for _, x := range rs {
		switch x.Status {
		case matching.Eligible:
			gs[0].Items = append(gs[0].Items, x)
		case matching.Potential:
			gs[1].Items = append(gs[1].Items, x)
		case matching.Unverified:
			gs[2].Items = append(gs[2].Items, x)
		default:
			nots = append(nots, x)
		}
	}
	if logIt {
		top := ""
		if len(rs) > 0 {
			top = rs[0].CampaignID
		}
		s.st.LogSearch(sid(r), p, segIDs, [4]int{cnt[matching.Eligible], cnt[matching.Potential], cnt[matching.Unverified], cnt[matching.NotElig]}, top)
	}
	return map[string]any{
		"P": p, "Results": rs, "Counts": counts, "Segments": segs, "Groups": gs, "Nots": nots,
		"Adverse": matching.AdverseCredit(p.Credit), "CreditText": creditText[p.Credit],
		"Query": q(profileQuery(p)),
	}, nil
}

func targets(campaignSegs, profileSegs []string) bool {
	if len(campaignSegs) == 0 {
		return true
	}
	for _, a := range campaignSegs {
		for _, b := range profileSegs {
			if a == b {
				return true
			}
		}
	}
	return false
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	p := ParseProfile(r.URL.Query())
	data, err := s.results(r, p, len(r.URL.Query()) > 0)
	if err != nil {
		s.fail(w, err)
		return
	}
	partners, err := s.st.Partners()
	if err != nil {
		s.fail(w, err)
		return
	}
	for k, v := range s.base(r, "3Credit – Tìm sản phẩm tài chính phù hợp") {
		data[k] = v
	}
	data["Partners"] = partners
	data["Posts"], _ = s.st.Posts(true, 4)
	data["Eco"], data["Network"], data["Parent"] = store.Ecosystem, store.Network, store.Parent
	data["Back"], data["Source"] = "/?"+profileQuery(p).Encode(), "trang-chu"
	if msg, kind := s.takeFlash(w, r); msg != "" {
		data["Flash"], data["FlashKind"] = msg, kind
	}
	data["Needs"], data["Regions"], data["Jobs"], data["Incomes"] = needs, regions, jobs, incomes
	data["Channels"], data["Credits"], data["Urgencies"], data["Vehicles"] = channels, credits, urgencies, vehicles
	s.render(w, "home.html", data)
}

func (s *Server) apiMatch(w http.ResponseWriter, r *http.Request) {
	data, err := s.results(r, ParseProfile(r.URL.Query()), true)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, "results", data)
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	o, err := s.st.Offer(r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && o.Campaign.Status != model.StatusActive && o.Campaign.Status != model.StatusPending) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	p := ParseProfile(r.URL.Query())
	res := matching.Evaluate(o, p, o.Campaign.Live(s.now()))
	data := s.base(r, o.Product.Name+" · "+o.Partner.Name+" – 3Credit")
	data["R"], data["Query"] = res, q(profileQuery(p))
	data["Back"], data["Source"], data["CampaignID"] = "/san-pham/"+o.Campaign.ID+"?"+profileQuery(p).Encode(), "san-pham", o.Campaign.ID
	if msg, kind := s.takeFlash(w, r); msg != "" {
		data["Flash"], data["FlashKind"] = msg, kind
	}
	s.render(w, "detail.html", data)
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	p := ParseProfile(r.URL.Query())
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	if len(ids) > 3 {
		ids = ids[:3]
	}
	var items []matching.Result
	for _, id := range ids {
		o, err := s.st.Offer(strings.TrimSpace(id))
		if err != nil || (o.Campaign.Status != model.StatusActive && o.Campaign.Status != model.StatusPending) {
			continue
		}
		items = append(items, matching.Evaluate(o, p, o.Campaign.Live(s.now())))
	}
	if len(items) == 0 {
		http.Redirect(w, r, "/#tim", http.StatusSeeOther)
		return
	}
	data := s.base(r, "So sánh sản phẩm – 3Credit")
	data["Items"], data["Query"] = items, q(profileQuery(p))
	s.render(w, "compare.html", data)
}

func device(ua string) string {
	ua = strings.ToLower(ua)
	if strings.Contains(ua, "mobi") || strings.Contains(ua, "android") || strings.Contains(ua, "iphone") {
		return "mobile"
	}
	return "desktop"
}

func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustCF {
		if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" && len(ip) <= 45 {
			return ip
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}

func (s *Server) goClick(w http.ResponseWriter, r *http.Request) {
	o, err := s.st.Offer(r.PathValue("id"))
	if err != nil || !o.Campaign.Live(s.now()) {
		http.Redirect(w, r, "/#tim", http.StatusSeeOther)
		return
	}
	ref, _ := url.Parse(r.Referer())
	landing := ""
	if ref != nil {
		landing = ref.Path
	}
	clickID, dest, err := s.tr.Click(o, tracking.ClickMeta{
		Session: sid(r), Source: r.URL.Query().Get("utm_source"), Medium: r.URL.Query().Get("utm_medium"),
		Landing: landing, Device: device(r.UserAgent()), IP: s.clientIP(r),
	})
	if err != nil {
		log.Printf("click %s: %v", o.Campaign.ID, err)
		http.Redirect(w, r, "/#tim", http.StatusSeeOther)
		return
	}
	data := s.base(r, "Chuyển sang "+o.Partner.Name+" – 3Credit")
	data["O"], data["ClickID"], data["URL"] = o, clickID, template.URL(dest)
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, "go.html", data)
}

func (s *Server) leadSubmit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil || !s.checkCSRF(r) {
		http.Error(w, "Phiên đã hết hạn, vui lòng tải lại trang", http.StatusForbidden)
		return
	}
	pv, _ := url.ParseQuery(r.FormValue("profile"))
	p := ParseProfile(pv)
	back := func(msg string, ok bool) {
		s.setFlash(w, msg, ok)
		http.Redirect(w, r, "/san-pham/"+url.PathEscape(id)+"?"+profileQuery(p).Encode(), http.StatusSeeOther)
	}
	o, err := s.st.Offer(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_, err = s.lr.Submit(o, sid(r), r.FormValue("consent") == "yes", leads.Payload{
		Name: r.FormValue("name"), Phone: r.FormValue("phone"), Amount: p.Amount, Term: p.Term, Profile: p,
	})
	if err != nil {
		back(err.Error(), false)
		return
	}
	back("Đã gửi thông tin cho "+o.Partner.Name+". Đối tác sẽ liên hệ bạn.", true)
}

func (s *Server) postback(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, err := s.tr.Postback(r.PathValue("network"), r.Form)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false}`))
		return
	}
	w.Write([]byte(`{"ok":true,"result":"` + res.Result + `"}`))
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	log.Printf("error: %v", err)
	http.Error(w, "Lỗi hệ thống, vui lòng thử lại", http.StatusInternalServerError)
}

// Label returns the Vietnamese label for a profile code (need, job, region, credit…).
func Label(code string) string {
	for _, list := range [][]opt{needs, regions, jobs, channels, credits, urgencies, vehicles} {
		for _, o := range list {
			if o.V == code && code != "" {
				return o.L
			}
		}
	}
	return code
}
