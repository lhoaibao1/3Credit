package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/model"
	"github.com/threecredit/platform/internal/store"
)

// ---------- inquiries (khách cần tư vấn) ----------

func (s *Server) adminInquiries(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	list, err := s.lr.Inquiries(st, 500)
	if err != nil {
		s.fail(w, err)
		return
	}
	counts := map[string]int{}
	for _, x := range leads.InquiryStatuses {
		counts[x] = s.st.Count(`SELECT COUNT(*) FROM inquiries WHERE status=?`, x)
	}
	d := s.adminData(w, r, "inquiries", "Khách cần tư vấn")
	d["List"], d["Statuses"], d["Filter"], d["Counts"] = list, leads.InquiryStatuses, st, counts
	d["StatusLabel"] = inquiryLabels
	s.st.Audit(s.actor(r), "view", "inquiries", st, fmt.Sprintf("%d rows", len(list)))
	s.render(w, "admin_inquiries", d)
}

var inquiryLabels = map[string]string{"new": "Mới", "contacted": "Đã gọi", "qualified": "Đủ điều kiện", "sent_to_partner": "Đã chuyển đối tác", "closed": "Đóng"}

func (s *Server) adminInquiryUpdate(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err := s.lr.UpdateInquiry(id, r.FormValue("status"), strings.TrimSpace(r.FormValue("note"))); err != nil {
		s.back(w, r, "/admin/inquiries", errText(err), false)
		return
	}
	s.st.Audit(s.actor(r), "update", "inquiry", strconv.FormatInt(id, 10), r.FormValue("status"))
	s.back(w, r, "/admin/inquiries?status="+r.FormValue("filter"), fmt.Sprintf("Đã cập nhật yêu cầu #%d", id), true)
}

func (s *Server) adminInquiriesCSV(w http.ResponseWriter, r *http.Request) {
	st := r.URL.Query().Get("status")
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="3credit-khach-tu-van-`+time.Now().Format("20060102-1504")+`.csv"`)
	s.st.Audit(s.actor(r), "export", "inquiries", st, "csv")
	if err := s.lr.ExportCSV(w, st, Label); err != nil {
		s.fail(w, err)
	}
}

// ---------- posts ----------

func (s *Server) adminPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := s.st.Posts(false, 500)
	if err != nil {
		s.fail(w, err)
		return
	}
	d := s.adminData(w, r, "posts", "Bài viết")
	d["Posts"] = posts
	s.render(w, "admin_posts", d)
}

func (s *Server) adminPostEdit(w http.ResponseWriter, r *http.Request) {
	p := store.Post{Category: "Kiến thức"}
	if id := r.PathValue("id"); id != "new" {
		n, _ := strconv.ParseInt(id, 10, 64)
		var err error
		if p, err = s.st.PostByID(n); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	d := s.adminData(w, r, "posts", "Soạn bài viết")
	d["P"] = p
	s.render(w, "admin_post_edit", d)
}

func (s *Server) adminPostSave(w http.ResponseWriter, r *http.Request) {
	idS := r.PathValue("id")
	var p store.Post
	if idS != "new" {
		n, _ := strconv.ParseInt(idS, 10, 64)
		old, err := s.st.PostByID(n)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		p = old
	}
	if r.FormValue("delete") == "1" && p.ID != 0 {
		s.st.DeletePost(p.ID, s.actor(r))
		s.back(w, r, "/admin/posts", "Đã xóa bài viết", true)
		return
	}
	p.Title, p.Slug = r.FormValue("title"), r.FormValue("slug")
	p.Summary, p.Body, p.Category = strings.TrimSpace(r.FormValue("summary")), r.FormValue("body"), strings.TrimSpace(r.FormValue("category"))
	p.Published = r.FormValue("published") == "1"
	if path, err := s.saveUpload(r, "cover", "posts", 600<<10); err != nil {
		s.back(w, r, "/admin/posts/"+idS, errText(err), false)
		return
	} else if path != "" {
		p.CoverPath = path
	}
	id, err := s.st.SavePost(p, s.actor(r))
	if err != nil {
		s.back(w, r, "/admin/posts/"+idS, "Chưa lưu: "+errText(err), false)
		return
	}
	s.back(w, r, fmt.Sprintf("/admin/posts/%d", id), "Đã lưu bài viết", true)
}

// saveUpload stores an image from a multipart field. Returns "" when no file was sent.
func (s *Server) saveUpload(r *http.Request, field, dir string, max int64) (string, error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return "", nil
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || int64(len(body)) > max {
		return "", fmt.Errorf("ảnh vượt quá %d KB", max>>10)
	}
	ext := ""
	switch ct := http.DetectContentType(body); {
	case ct == "image/png":
		ext = ".png"
	case ct == "image/jpeg":
		ext = ".jpg"
	case ct == "image/webp":
		ext = ".webp"
	case strings.HasSuffix(strings.ToLower(hdr.Filename), ".svg") && bytes.Contains(bytes.ToLower(body), []byte("<svg")):
		if badSVG.Match(body) {
			return "", fmt.Errorf("SVG chứa script hoặc liên kết ngoài, đã từ chối")
		}
		ext = ".svg"
	default:
		return "", fmt.Errorf("chỉ nhận ảnh PNG, JPG, WebP hoặc SVG")
	}
	full := filepath.Join(s.cfg.DataDir, "uploads", dir)
	if err := ensureDir(full); err != nil {
		return "", err
	}
	name := strconv.FormatInt(time.Now().UnixNano(), 36) + ext
	if err := os.WriteFile(filepath.Join(full, name), body, 0o644); err != nil {
		return "", err
	}
	return "/uploads/" + dir + "/" + name, nil
}

// ---------- settings ----------

func (s *Server) adminSettings(w http.ResponseWriter, r *http.Request) {
	d := s.adminData(w, r, "settings", "Cài đặt")
	d["Keys"], d["Vals"] = store.SettingKeys, s.st.Settings()
	s.render(w, "admin_settings", d)
}

func (s *Server) adminSettingsSave(w http.ResponseWriter, r *http.Request) {
	m := map[string]string{}
	for _, k := range store.SettingKeys {
		m[k.Key] = r.FormValue(k.Key)
	}
	if err := s.st.SaveSettings(m, s.actor(r)); err != nil {
		s.back(w, r, "/admin/settings", errText(err), false)
		return
	}
	s.back(w, r, "/admin/settings", "Đã lưu. Thông tin hiển thị ngay ở chân trang và các trang pháp lý.", true)
}

// ---------- deletion requests ----------

func (s *Server) adminDeletions(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.DeletionRequests()
	if err != nil {
		s.fail(w, err)
		return
	}
	d := s.adminData(w, r, "deletions", "Yêu cầu xóa dữ liệu")
	d["List"] = list
	s.render(w, "admin_deletions", d)
}

func (s *Server) adminDeletionDo(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var hash string
	if err := s.st.DB.QueryRow(`SELECT phone_hash FROM deletion_requests WHERE id=?`, id).Scan(&hash); err != nil {
		http.NotFound(w, r)
		return
	}
	n, err := s.lr.EraseByPhone(hash)
	if err != nil {
		s.back(w, r, "/admin/deletions", errText(err), false)
		return
	}
	s.st.DB.Exec(`UPDATE deletion_requests SET status='done' WHERE id=?`, id)
	s.st.Audit(s.actor(r), "erase", "personal_data", strconv.FormatInt(id, 10), fmt.Sprintf("%d records", n))
	s.back(w, r, "/admin/deletions", fmt.Sprintf("Đã xóa %d bản ghi của số điện thoại này", n), true)
}

// ---------- campaigns: quick switch, new product, new partner ----------

func (s *Server) adminCampaignStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.st.SetCampaignStatus(id, r.FormValue("status"), s.actor(r)); err != nil {
		msg := errText(err)
		if strings.Contains(msg, "tracking URL") {
			msg = id + ": cần dán tracking URL trước khi bật. Bấm Sửa để thêm link."
		}
		s.back(w, r, "/admin/campaigns", msg, false)
		return
	}
	s.back(w, r, "/admin/campaigns", "Đã chuyển "+id+" sang "+r.FormValue("status"), true)
}

func (s *Server) adminProductNew(w http.ResponseWriter, r *http.Request) {
	cid := strings.TrimSpace(r.FormValue("campaign_id"))
	if cid != "" && !idRe.MatchString(cid) {
		s.back(w, r, "/admin/campaigns", "Mã chiến dịch chỉ gồm chữ, số, dấu gạch ngang", false)
		return
	}
	if err := s.st.CreateProduct(r.FormValue("partner_id"), r.FormValue("name"), r.FormValue("category"), cid, s.actor(r)); err != nil {
		s.back(w, r, "/admin/campaigns", "Chưa tạo: "+errText(err), false)
		return
	}
	s.back(w, r, "/admin/campaigns", "Đã tạo sản phẩm và chiến dịch (Draft). Bấm Chính sách để nhập điều kiện, Sửa để gắn link.", true)
}

func (s *Server) adminPartnerNew(w http.ResponseWriter, r *http.Request) {
	p := model.Partner{ID: strings.TrimSpace(r.FormValue("id")), Name: r.FormValue("name"), LegalName: strings.TrimSpace(r.FormValue("legal_name")),
		OrgType: strings.TrimSpace(r.FormValue("org_type")), Group: r.FormValue("group")}
	if err := s.st.CreatePartner(p, s.actor(r)); err != nil {
		s.back(w, r, "/admin/partners", "Chưa tạo: "+errText(err), false)
		return
	}
	s.back(w, r, "/admin/partners", "Đã thêm đối tác "+p.Name, true)
}

// ---------- policy form (no JSON) ----------

type PolicyForm struct {
	AmountMin, AmountMax, AmountConf string
	TermMin, TermMax, TermConf       string
	AgeMin, AgeMax, AgeConf, AgeNote string
	IncomeMin, IncomeCivil, IncConf  string
	ChannelConf                      string
	Jobs                             []string
	JobsMode, JobsWhy, JobsConf      string
	VehicleConf                      string
	Credit                           []string
	CreditConf                       string
	Facts                            []FactForm
	Flags, Source                    string
}

type FactForm struct{ Key, Label, Text, Conf string }

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }
func conf(c model.Confidence) string {
	if c == "" {
		return "U"
	}
	return string(c)
}

func toForm(v model.PolicyVersion) PolicyForm {
	R, f := v.Rules, PolicyForm{Source: v.Source, Flags: strings.Join(v.Flags, "\n"), JobsMode: "allow"}
	if R.Amount != nil {
		f.AmountMin, f.AmountMax, f.AmountConf = num(R.Amount.Min/1e6), num(R.Amount.Max/1e6), conf(R.Amount.Conf)
	}
	if R.Term != nil {
		f.TermMin, f.TermMax, f.TermConf = num(R.Term.Min), num(R.Term.Max), conf(R.Term.Conf)
	}
	if R.Age != nil {
		f.AgeMin, f.AgeMax, f.AgeConf, f.AgeNote = num(R.Age.Min), num(R.Age.Max), conf(R.Age.Conf), R.Age.Note
	}
	if R.Income != nil {
		f.IncomeMin, f.IncConf = num(R.Income.Min), conf(R.Income.Conf)
		if R.Income.CivilMin > 0 {
			f.IncomeCivil = num(R.Income.CivilMin)
		}
	}
	if R.Channel != nil {
		f.ChannelConf = conf(R.Channel.Conf)
	}
	if R.Jobs != nil {
		f.Jobs, f.JobsMode, f.JobsWhy, f.JobsConf = R.Jobs.Values, R.Jobs.Mode, R.Jobs.Why, conf(R.Jobs.Conf)
	}
	if R.Vehicle != nil && R.Vehicle.Required {
		f.VehicleConf = conf(R.Vehicle.Conf)
	}
	if R.Credit != nil {
		f.Credit, f.CreditConf = R.Credit.Values, conf(R.Credit.Conf)
	}
	I := v.Info
	for _, x := range []struct {
		k, l string
		p    *model.Fact
	}{{"rate", "Lãi suất / chi phí", I.Rate}, {"amount", "Hạn mức", I.Amount}, {"term", "Kỳ hạn", I.Term}, {"fee", "Phí", I.Fee}, {"docs", "Hồ sơ, điều kiện khác", I.Docs}, {"speed", "Thời gian xử lý", I.Speed}} {
		ff := FactForm{Key: x.k, Label: x.l}
		if x.p != nil {
			ff.Text, ff.Conf = x.p.Text, conf(x.p.Conf)
		}
		f.Facts = append(f.Facts, ff)
	}
	return f
}

func pconf(s string) (model.Confidence, bool) {
	switch s {
	case "A", "B", "C":
		return model.Confidence(s), true
	case "U":
		return "", true
	}
	return "", false // "" = rule not used
}

func pf(r *http.Request, k string) (float64, bool) {
	v := strings.TrimSpace(strings.ReplaceAll(r.FormValue(k), ",", "."))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil && f >= 0
}

func fromForm(r *http.Request) (model.Rules, model.Info, error) {
	var R model.Rules
	rng := func(name, label string, scale float64) (*model.Range, error) {
		c, used := pconf(r.FormValue(name + "_conf"))
		if !used {
			return nil, nil
		}
		mn, _ := pf(r, name+"_min")
		mx, ok := pf(r, name+"_max")
		if !ok || mx < mn {
			return nil, fmt.Errorf("%s: cần giá trị tối đa và tối đa ≥ tối thiểu", label)
		}
		return &model.Range{Min: mn * scale, Max: mx * scale, Conf: c}, nil
	}
	var err error
	if R.Amount, err = rng("amount", "Hạn mức", 1e6); err != nil {
		return R, model.Info{}, err
	}
	if R.Term, err = rng("term", "Kỳ hạn", 1); err != nil {
		return R, model.Info{}, err
	}
	if R.Age, err = rng("age", "Độ tuổi", 1); err != nil {
		return R, model.Info{}, err
	}
	if R.Age != nil {
		R.Age.Note = strings.TrimSpace(r.FormValue("age_note"))
	}
	if c, used := pconf(r.FormValue("income_conf")); used {
		mn, ok := pf(r, "income_min")
		if !ok {
			return R, model.Info{}, fmt.Errorf("Thu nhập: cần mức tối thiểu")
		}
		civ, _ := pf(r, "income_civil")
		R.Income = &model.IncomeRule{Min: mn, CivilMin: civ, Conf: c}
	}
	if c, used := pconf(r.FormValue("channel_conf")); used {
		R.Channel = &model.ListRule{Values: []string{"bank_transfer"}, Conf: c}
	}
	if c, used := pconf(r.FormValue("jobs_conf")); used {
		jobsSel := r.Form["jobs"]
		if len(jobsSel) == 0 {
			return R, model.Info{}, fmt.Errorf("Nghề nghiệp: chọn ít nhất một nhóm")
		}
		mode := r.FormValue("jobs_mode")
		if mode != "boost" {
			mode = "allow"
		}
		why := strings.TrimSpace(r.FormValue("jobs_why"))
		if why == "" {
			why = "Sản phẩm dành cho nhóm nghề đã chọn"
		}
		R.Jobs = &model.ListRule{Values: jobsSel, Mode: mode, Why: why, Conf: c}
	}
	if c, used := pconf(r.FormValue("vehicle_conf")); used {
		R.Vehicle = &model.FlagRule{Required: true, Conf: c}
	}
	if c, used := pconf(r.FormValue("credit_conf")); used {
		cs := r.Form["credit"]
		if len(cs) == 0 {
			return R, model.Info{}, fmt.Errorf("Lịch sử tín dụng: chọn ít nhất một trường hợp được nhận")
		}
		R.Credit = &model.ListRule{Values: cs, Conf: c}
	}
	var I model.Info
	fact := func(k string) *model.Fact {
		t := strings.TrimSpace(r.FormValue("info_" + k))
		if t == "" {
			return nil
		}
		c, _ := pconf(r.FormValue("info_" + k + "_conf"))
		return &model.Fact{Text: t, Conf: c}
	}
	I.Rate, I.Amount, I.Term, I.Fee, I.Docs, I.Speed = fact("rate"), fact("amount"), fact("term"), fact("fee"), fact("docs"), fact("speed")
	return R, I, nil
}
