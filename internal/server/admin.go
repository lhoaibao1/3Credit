package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/threecredit/platform/internal/matching"
	"github.com/threecredit/platform/internal/model"
	"github.com/threecredit/platform/internal/store"
)

func (s *Server) adminData(w http.ResponseWriter, r *http.Request, page, title string) map[string]any {
	d := s.base(r, title+" – 3Credit Admin")
	d["Admin"], d["Page"], d["User"], d["SiteURL"], d["Build"] = true, page, s.actor(r), s.cfg.BaseURL, s.cfg.Version
	d["Title"] = title
	if msg, kind := s.takeFlash(w, r); msg != "" {
		d["Flash"], d["FlashKind"] = msg, kind
	}
	return d
}

func (s *Server) back(w http.ResponseWriter, r *http.Request, to, msg string, ok bool) {
	s.setFlash(w, msg, ok)
	http.Redirect(w, r, to, http.StatusSeeOther)
}

type kpis struct {
	Campaigns, Partners, Live, Clicks, Leads, Searches, Conversions int
	Commission                                                      float64
	CR                                                              string
	NoURL, NoSeg, LowData, CreditVerified, NoLogo, NoSecret         int
	Inquiries, NewInquiries                                         int
}

func (s *Server) adminOverview(w http.ResponseWriter, r *http.Request) {
	offers, err := s.st.Offers()
	if err != nil {
		s.fail(w, err)
		return
	}
	partners, _ := s.st.Partners()
	k := kpis{Campaigns: len(offers), Partners: len(partners)}
	now := s.now()
	credit := map[string]bool{}
	for _, o := range offers {
		if o.Campaign.Live(now) {
			k.Live++
			if o.Campaign.PostbackSecret == "" {
				k.NoSecret++
			}
			if len(o.Campaign.TargetSegments) == 0 {
				k.NoSeg++
			}
		}
		if o.Campaign.TrackingURL == "" {
			k.NoURL++
		}
		if verifiedPct(o) < 50 {
			k.LowData++
		}
		if c := o.Policy.Rules.Credit; c != nil && c.Conf.Verified() {
			credit[o.Partner.ID] = true
		}
	}
	k.CreditVerified = len(credit)
	for _, p := range partners {
		if p.LogoPath == "" {
			k.NoLogo++
		}
	}
	k.Inquiries = s.st.Count(`SELECT COUNT(*) FROM inquiries`)
	k.NewInquiries = s.st.Count(`SELECT COUNT(*) FROM inquiries WHERE status='new'`)
	k.Clicks = s.st.Count(`SELECT COUNT(*) FROM clicks`)
	k.Leads = s.st.Count(`SELECT COUNT(*) FROM leads`)
	k.Searches = s.st.Count(`SELECT COUNT(*) FROM searches`)
	k.Conversions = s.st.Count(`SELECT COUNT(*) FROM conversions WHERE status='approved'`)
	k.Commission = s.st.Sum(`SELECT COALESCE(SUM(payout),0) FROM conversions WHERE status='approved'`)
	k.CR = "—"
	if k.Clicks > 0 {
		k.CR = fmt.Sprintf("%.1f%%", float64(k.Conversions)*100/float64(k.Clicks))
	}
	d := s.adminData(w, r, "overview", "Tổng quan")
	d["K"] = k
	s.render(w, "admin_overview", d)
}

func (s *Server) adminCampaigns(w http.ResponseWriter, r *http.Request) {
	offers, err := s.st.Offers()
	if err != nil {
		s.fail(w, err)
		return
	}
	partners, _ := s.st.Partners()
	d := s.adminData(w, r, "campaigns", "Chiến dịch")
	d["Offers"], d["Partners"], d["Categories"] = offers, partners, store.Categories
	s.render(w, "admin_campaigns", d)
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9-]{3,32}$`)

func (s *Server) adminCampaignNew(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.FormValue("id"))
	if !idRe.MatchString(id) {
		s.back(w, r, "/admin/campaigns", "Mã chiến dịch chỉ gồm chữ, số, dấu gạch ngang (3–32 ký tự)", false)
		return
	}
	if err := s.st.CreateCampaign(id, r.FormValue("product_id"), strings.TrimSpace(r.FormValue("name")), s.actor(r)); err != nil {
		s.back(w, r, "/admin/campaigns", "Không tạo được: "+errText(err), false)
		return
	}
	s.back(w, r, "/admin/campaigns/"+id, "Đã tạo chiến dịch "+id+" (Draft)", true)
}

func (s *Server) adminCampaignEdit(w http.ResponseWriter, r *http.Request) {
	o, err := s.st.Offer(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	segs, _ := s.st.Segments()
	d := s.adminData(w, r, "campaigns", "Sửa chiến dịch")
	d["O"], d["Segments"], d["Statuses"] = o, segs, model.CampaignStatuses
	d["Models"] = []string{"", "CPL", "CPA", "CPS", "RevShare"}
	d["Base"] = s.cfg.BaseURL
	s.render(w, "admin_campaign_edit", d)
}

func parseLocal(v string) *time.Time {
	if v == "" {
		return nil
	}
	t, err := time.ParseInLocation("2006-01-02T15:04", v, time.Local)
	if err != nil {
		return nil
	}
	return &t
}

func (s *Server) adminCampaignSave(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.st.Campaign(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	c.Name = strings.TrimSpace(r.FormValue("name"))
	c.Status = r.FormValue("status")
	c.Priority, _ = strconv.Atoi(r.FormValue("priority"))
	c.TrackingURL = strings.TrimSpace(r.FormValue("tracking_url"))
	c.SubIDParam = strings.TrimSpace(r.FormValue("subid_param"))
	if v := r.FormValue("postback_secret"); v != "" {
		c.PostbackSecret = v
	}
	if c.PostbackSecret == "" && c.TrackingURL != "" { // a link without a secret could never record conversions
		b := make([]byte, 16)
		rand.Read(b)
		c.PostbackSecret = hex.EncodeToString(b)
	}
	c.CommissionModel = r.FormValue("commission_model")
	c.CommissionValue, _ = strconv.ParseFloat(r.FormValue("commission_value"), 64)
	c.DailyLeadCap, _ = strconv.Atoi(r.FormValue("daily_lead_cap"))
	c.TotalLeadCap, _ = strconv.Atoi(r.FormValue("total_lead_cap"))
	c.StartAt, c.EndAt = parseLocal(r.FormValue("start_at")), parseLocal(r.FormValue("end_at"))
	c.TargetSegments = r.Form["segments"]
	c.LeadEndpoint = strings.TrimSpace(r.FormValue("lead_endpoint"))
	if v := r.FormValue("lead_auth_header"); v != "" {
		c.LeadAuthHeader = v
	}
	if c.DailyLeadCap < 0 || c.TotalLeadCap < 0 || c.CommissionValue < 0 {
		s.back(w, r, "/admin/campaigns/"+id, "Giá trị không được âm", false)
		return
	}
	if err := s.st.SaveCampaign(c, s.actor(r)); err != nil {
		s.back(w, r, "/admin/campaigns/"+id, "Chưa lưu: "+errText(err), false)
		return
	}
	s.back(w, r, "/admin/campaigns/"+id, "Đã lưu "+id+". Thay đổi có hiệu lực ngay.", true)
}

func (s *Server) adminPartners(w http.ResponseWriter, r *http.Request) {
	partners, err := s.st.Partners()
	if err != nil {
		s.fail(w, err)
		return
	}
	offers, _ := s.st.Offers()
	d := s.adminData(w, r, "partners", "Đối tác & chính sách")
	d["Partners"], d["Offers"] = partners, offers
	s.render(w, "admin_partners", d)
}

var badSVG = regexp.MustCompile(`(?i)<script|\bon[a-z]+\s*=|javascript:|<foreignobject|<iframe|<embed|<object|xlink:href\s*=\s*["']\s*(?:https?:|data:)`)

func (s *Server) adminPartnerSave(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	partners, _ := s.st.Partners()
	var p *model.Partner
	for i := range partners {
		if partners[i].ID == id {
			p = &partners[i]
		}
	}
	if p == nil {
		http.NotFound(w, r)
		return
	}
	p.LegalName = strings.TrimSpace(r.FormValue("legal_name"))
	p.LegalConf = model.Confidence(map[string]string{"A": "A", "B": "B", "C": "C"}[r.FormValue("legal_conf")])
	switch v := r.FormValue("integration"); v {
	case "pending", "redirect", "postback", "live":
		p.Integration = v
	}
	if f, hdr, err := r.FormFile("logo"); err == nil {
		defer f.Close()
		body, err := io.ReadAll(io.LimitReader(f, 200<<10+1))
		if err != nil || len(body) > 200<<10 {
			s.back(w, r, "/admin/partners", "Logo vượt quá 200 KB", false)
			return
		}
		ext := ""
		switch ct := http.DetectContentType(body); {
		case ct == "image/png":
			ext = ".png"
		case ct == "image/webp":
			ext = ".webp"
		case strings.HasSuffix(strings.ToLower(hdr.Filename), ".svg") && bytes.Contains(bytes.ToLower(body), []byte("<svg")):
			if badSVG.Match(body) {
				s.back(w, r, "/admin/partners", "SVG chứa script hoặc liên kết ngoài, đã từ chối", false)
				return
			}
			ext = ".svg"
		default:
			s.back(w, r, "/admin/partners", "Chỉ nhận PNG, SVG hoặc WebP", false)
			return
		}
		if err := ensureDir(s.logoDir()); err != nil {
			s.fail(w, err)
			return
		}
		name := id + ext
		if err := os.WriteFile(filepath.Join(s.logoDir(), name), body, 0o644); err != nil {
			s.fail(w, err)
			return
		}
		p.LogoPath = "/uploads/partners/" + name + "?v=" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	if err := s.st.UpdatePartner(*p, s.actor(r)); err != nil {
		s.back(w, r, "/admin/partners", "Chưa lưu: "+errText(err), false)
		return
	}
	s.back(w, r, "/admin/partners", "Đã lưu "+p.Name, true)
}

func (s *Server) adminPolicy(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("product")
	hist, err := s.st.PolicyHistory(pid)
	if err != nil || len(hist) == 0 {
		http.NotFound(w, r)
		return
	}
	offers, _ := s.st.Offers()
	var name string
	for _, o := range offers {
		if o.Product.ID == pid {
			name = o.Partner.Name + " · " + o.Product.Name
		}
	}
	d := s.adminData(w, r, "campaigns", "Chính sách sản phẩm")
	d["ProductID"], d["ProductName"], d["History"], d["F"] = pid, name, hist, toForm(hist[0])
	d["Today"], d["Jobs"], d["Credits"] = s.now().Format("2006-01-02"), jobs, credits[1:]
	s.render(w, "admin_policy", d)
}

func (s *Server) adminPolicySave(w http.ResponseWriter, r *http.Request) {
	pid := r.PathValue("product")
	to := "/admin/policies/" + pid
	rules, info, err := fromForm(r)
	if err != nil {
		s.back(w, r, to, "Chưa lưu: "+errText(err), false)
		return
	}
	v := model.PolicyVersion{ProductID: pid, Rules: rules, Info: info}
	for _, l := range strings.Split(r.FormValue("flags"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			v.Flags = append(v.Flags, l)
		}
	}
	v.Source, v.VerifiedOn = strings.TrimSpace(r.FormValue("source")), strings.TrimSpace(r.FormValue("verified_on"))
	v.ChangedBy, v.ChangeNote = s.actor(r), strings.TrimSpace(r.FormValue("change_note"))
	if v.ChangeNote == "" {
		s.back(w, r, to, "Cần ghi nội dung thay đổi", false)
		return
	}
	n, err := s.st.AddPolicyVersion(v)
	if err != nil {
		s.back(w, r, to, "Chưa lưu: "+errText(err), false)
		return
	}
	s.back(w, r, to, fmt.Sprintf("Đã lưu phiên bản v%d. Kết quả đề xuất cho khách cập nhật ngay.", n), true)
}

func (s *Server) adminSegments(w http.ResponseWriter, r *http.Request) {
	segs, err := s.st.Segments()
	if err != nil {
		s.fail(w, err)
		return
	}
	d := s.adminData(w, r, "segments", "Segment khách hàng")
	d["Segments"] = segs
	s.render(w, "admin_segments", d)
}

var segRe = regexp.MustCompile(`^[A-Z0-9_-]{3,20}$`)

func (s *Server) adminSegmentSave(w http.ResponseWriter, r *http.Request) {
	g := model.Segment{ID: strings.TrimSpace(r.FormValue("id")), Name: strings.TrimSpace(r.FormValue("name")), Rule: r.FormValue("rule"), Active: r.FormValue("active") == "1"}
	if !segRe.MatchString(g.ID) || g.Name == "" {
		s.back(w, r, "/admin/segments", "Mã segment gồm chữ in hoa, số, _ hoặc - (3–20 ký tự) và cần có tên", false)
		return
	}
	if err := s.st.SaveSegment(g, s.actor(r)); err != nil {
		s.back(w, r, "/admin/segments", errText(err), false)
		return
	}
	s.back(w, r, "/admin/segments", "Đã lưu "+g.ID, true)
}

type presetResult struct {
	Name   string
	Counts map[string]int
	Top    *matching.Result
}

func presets() []struct {
	Name string
	P    model.Profile
} {
	M := 1e6
	return []struct {
		Name string
		P    model.Profile
	}{
		{"NV văn phòng, 28t, 12tr, vay 50tr/24th", model.Profile{Need: "cash_loan", Amount: 50 * M, Term: 24, Age: 28, Job: "salaried_office", Income: 12, Channel: "bank_transfer", Credit: "good", Urgency: "flexible", Vehicle: "none"}},
		{"Công chức, 40t, 9tr, vay 100tr/36th", model.Profile{Need: "cash_loan", Amount: 100 * M, Term: 36, Age: 40, Job: "civil_servant", Income: 9, Channel: "bank_transfer", Credit: "good", Urgency: "flexible", Vehicle: "none"}},
		{"Công nhân, lương tiền mặt, vay 20tr/12th", model.Profile{Need: "cash_loan", Amount: 20 * M, Term: 12, Age: 30, Job: "worker", Income: 6, Channel: "cash", Credit: "good", Urgency: "this_week", Vehicle: "none"}},
		{"Tự kinh doanh, vay 100tr/36th", model.Profile{Need: "cash_loan", Amount: 100 * M, Term: 36, Age: 35, Job: "self_employed", Income: 20, Channel: "mixed", Credit: "good", Urgency: "flexible", Vehicle: "none"}},
		{"Chưa từng vay, 22t, vay 10tr/6th", model.Profile{Need: "cash_loan", Amount: 10 * M, Term: 6, Age: 22, Job: "salaried_office", Income: 9, Channel: "bank_transfer", Credit: "none", Urgency: "flexible", Vehicle: "none"}},
		{"Nợ xấu chưa tất toán, cần gấp 5tr", model.Profile{Need: "cash_loan", Amount: 5 * M, Term: 3, Age: 30, Job: "worker", Income: 6, Channel: "cash", Credit: "bad_unsettled", Urgency: "today", Vehicle: "none"}},
		{"Nợ xấu, có xe máy, vay bằng xe 15tr", model.Profile{Need: "secured", Amount: 15 * M, Term: 12, Age: 32, Job: "freelancer", Income: 9, Channel: "mixed", Credit: "bad_settled", Urgency: "today", Vehicle: "motorbike"}},
		{"Cần thẻ tín dụng, 26t, 20tr", model.Profile{Need: "credit_card", Amount: 20 * M, Term: 12, Age: 26, Job: "salaried_office", Income: 20, Channel: "bank_transfer", Credit: "good", Urgency: "flexible", Vehicle: "none"}},
	}
}

func (s *Server) adminMatching(w http.ResponseWriter, r *http.Request) {
	offers, err := s.st.Offers()
	if err != nil {
		s.fail(w, err)
		return
	}
	now := s.now()
	var out []presetResult
	for _, pr := range presets() {
		rs := matching.Match(offers, pr.P, func(c model.Campaign) bool { return c.Live(now) })
		cnt := map[string]int{}
		for k, v := range matching.Counts(rs) {
			cnt[string(k)] = v
		}
		x := presetResult{Name: pr.Name, Counts: cnt}
		if len(rs) > 0 {
			x.Top = &rs[0]
		}
		out = append(out, x)
	}
	stats, _ := s.st.SegmentStats()
	top, _ := s.st.TopRecommended(10)
	d := s.adminData(w, r, "matching", "Matching analytics")
	d["Presets"], d["SegStats"], d["Top"] = out, stats, top
	s.render(w, "admin_matching", d)
}

type clickRow struct {
	ID, Campaign, Partner, Source, Device string
	At                                    time.Time
}
type convRow struct {
	Network, ID, Click, Campaign, Status, PayoutStatus string
	Payout                                             float64
}
type pbRow struct {
	Network, Query, Result string
	OK                     bool
	At                     time.Time
}
type perfRow struct {
	Campaign            string
	Clicks, Conversions int
	Payout              float64
	CR                  string
}

func (s *Server) adminTracking(w http.ResponseWriter, r *http.Request) {
	db := s.st.DB
	var clicks []clickRow
	if rows, err := db.Query(`SELECT click_id,campaign_id,partner_id,source,device,created_at FROM clicks ORDER BY created_at DESC LIMIT 100`); err == nil {
		for rows.Next() {
			var c clickRow
			rows.Scan(&c.ID, &c.Campaign, &c.Partner, &c.Source, &c.Device, &c.At)
			clicks = append(clicks, c)
		}
		rows.Close()
	}
	var convs []convRow
	if rows, err := db.Query(`SELECT network,conversion_id,click_id,campaign_id,status,payout,payout_status FROM conversions ORDER BY id DESC LIMIT 100`); err == nil {
		for rows.Next() {
			var c convRow
			rows.Scan(&c.Network, &c.ID, &c.Click, &c.Campaign, &c.Status, &c.Payout, &c.PayoutStatus)
			convs = append(convs, c)
		}
		rows.Close()
	}
	var pbs []pbRow
	if rows, err := db.Query(`SELECT network,query,signature_ok,result,created_at FROM postback_logs ORDER BY id DESC LIMIT 100`); err == nil {
		for rows.Next() {
			var p pbRow
			rows.Scan(&p.Network, &p.Query, &p.OK, &p.Result, &p.At)
			pbs = append(pbs, p)
		}
		rows.Close()
	}
	var perf []perfRow
	if rows, err := db.Query(`SELECT c.campaign_id, COUNT(DISTINCT c.click_id),
	    COUNT(DISTINCT CASE WHEN v.status='approved' THEN v.id END), COALESCE(SUM(CASE WHEN v.status='approved' THEN v.payout END),0)
	  FROM clicks c LEFT JOIN conversions v ON v.click_id=c.click_id GROUP BY c.campaign_id ORDER BY 2 DESC`); err == nil {
		for rows.Next() {
			var p perfRow
			rows.Scan(&p.Campaign, &p.Clicks, &p.Conversions, &p.Payout)
			p.CR = "—"
			if p.Clicks > 0 {
				p.CR = fmt.Sprintf("%.1f%%", float64(p.Conversions)*100/float64(p.Clicks))
			}
			perf = append(perf, p)
		}
		rows.Close()
	}
	d := s.adminData(w, r, "tracking", "Tracking & đối soát")
	d["Clicks"], d["Convs"], d["PB"], d["Perf"] = clicks, convs, pbs, perf
	s.render(w, "admin_tracking", d)
}

func (s *Server) adminLeads(w http.ResponseWriter, r *http.Request) {
	ls, err := s.lr.List(200)
	if err != nil {
		s.fail(w, err)
		return
	}
	d := s.adminData(w, r, "leads", "Lead")
	d["Leads"] = ls
	s.render(w, "admin_leads", d)
}

func (s *Server) adminAudit(w http.ResponseWriter, r *http.Request) {
	a, err := s.st.AuditLog(200)
	if err != nil {
		s.fail(w, err)
		return
	}
	d := s.adminData(w, r, "audit", "Nhật ký thay đổi")
	d["Audit"] = a
	s.render(w, "admin_audit", d)
}
