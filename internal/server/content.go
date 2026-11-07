package server

import (
	"database/sql"
	"errors"
	"html"
	"html/template"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/store"
)

// ---------- about / ecosystem ----------

func (s *Server) about(w http.ResponseWriter, r *http.Request) {
	partners, _ := s.st.Partners()
	d := s.base(r, "Giới thiệu 3Credit – sản phẩm của 3RDVN Technology")
	d["Eco"], d["Services"], d["Partners"], d["Parent"] = store.Ecosystem, store.EcoServices, partners, store.Parent
	d["Description"] = "3Credit là sản phẩm so sánh tài chính thuộc hệ sinh thái 3RDVN Technology, cùng với 3RDHub Vietnam."
	s.render(w, "about.html", d)
}

// ---------- tiny markdown: ## heading, - list, **bold**, *italic*, paragraphs ----------

var (
	boldRe   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe = regexp.MustCompile(`\*([^*]+)\*`)
)

func inline(s string) string {
	s = html.EscapeString(s)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	return italicRe.ReplaceAllString(s, "<em>$1</em>")
}

// Markdown renders a small, safe subset. Input is escaped first.
func Markdown(src string) template.HTML {
	var b strings.Builder
	inList := false
	closeList := func() {
		if inList {
			b.WriteString("</ul>")
			inList = false
		}
	}
	for _, block := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 1 && lines[0] == "" {
			continue
		}
		for _, l := range lines {
			l = strings.TrimSpace(l)
			switch {
			case strings.HasPrefix(l, "### "):
				closeList()
				b.WriteString("<h3>" + inline(l[4:]) + "</h3>")
			case strings.HasPrefix(l, "## "):
				closeList()
				b.WriteString("<h2>" + inline(l[3:]) + "</h2>")
			case strings.HasPrefix(l, "- "):
				if !inList {
					b.WriteString("<ul>")
					inList = true
				}
				b.WriteString("<li>" + inline(l[2:]) + "</li>")
			case l == "":
			default:
				closeList()
				b.WriteString("<p>" + inline(l) + "</p>")
			}
		}
		closeList()
	}
	return template.HTML(b.String())
}

// ---------- public: news ----------

func (s *Server) newsList(w http.ResponseWriter, r *http.Request) {
	posts, err := s.st.Posts(true, 100)
	if err != nil {
		s.fail(w, err)
		return
	}
	d := s.base(r, "Kiến thức tài chính – 3Credit")
	d["Posts"] = posts
	s.render(w, "news_list.html", d)
}

func (s *Server) newsDetail(w http.ResponseWriter, r *http.Request) {
	p, err := s.st.PostBySlug(r.PathValue("slug"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !p.Published) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	more, _ := s.st.Posts(true, 4)
	d := s.base(r, p.Title+" – 3Credit")
	d["Post"], d["BodyHTML"], d["More"] = p, Markdown(p.Body), more
	d["Description"] = p.Summary
	d["Source"], d["Back"] = "bai-viet:"+p.Slug, "/kien-thuc/"+p.Slug
	if msg, kind := s.takeFlash(w, r); msg != "" {
		d["Flash"], d["FlashKind"] = msg, kind
	}
	s.render(w, "news_detail.html", d)
}

// ---------- public: consultation request (to 3Credit) ----------

func (s *Server) inquirySubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.checkCSRF(r) {
		http.Error(w, "Phiên đã hết hạn, vui lòng tải lại trang", http.StatusForbidden)
		return
	}
	back := r.FormValue("back")
	if !strings.HasPrefix(back, "/") || strings.HasPrefix(back, "//") {
		back = "/"
	}
	in := leads.InquiryInput{Name: r.FormValue("name"), Phone: r.FormValue("phone"), Message: r.FormValue("message"),
		CampaignID: r.FormValue("campaign_id"), SourcePage: r.FormValue("source")}
	if raw := r.FormValue("profile"); raw != "" { // only when the visitor actually filled the profile form
		pv, _ := url.ParseQuery(raw)
		p := ParseProfile(pv)
		in.Need, in.Amount, in.Term, in.Job, in.Income, in.Region, in.Credit = p.Need, p.Amount, p.Term, p.Job, p.Income, p.Region, p.Credit
		in.SocialIns, in.LifeIns = p.SocialIns, p.LifeIns
	}
	_, dup, err := s.lr.SubmitInquiry(sid(r), r.FormValue("consent") == "yes", in)
	if err != nil {
		s.setFlash(w, err.Error(), false)
	} else if dup {
		s.setFlash(w, "3Credit đã nhận yêu cầu của bạn hôm nay và sẽ gọi lại sớm.", true)
	} else {
		s.setFlash(w, "Đã nhận yêu cầu. Chuyên viên 3Credit sẽ gọi lại cho bạn trong giờ làm việc.", true)
	}
	http.Redirect(w, r, back+"#tu-van", http.StatusSeeOther)
}

// ---------- public: legal pages ----------

var legalPages = map[string]string{
	"/dieu-khoan":       "Điều khoản sử dụng",
	"/tieu-chi-doi-tac": "Tiêu chí lựa chọn đối tác",
	"/xoa-du-lieu":      "Yêu cầu xóa dữ liệu cá nhân",
}

func (s *Server) legal(w http.ResponseWriter, r *http.Request) {
	title := legalPages[r.URL.Path]
	d := s.base(r, title+" – 3Credit")
	d["Page"], d["PageTitle"] = strings.TrimPrefix(r.URL.Path, "/"), title
	if msg, kind := s.takeFlash(w, r); msg != "" {
		d["Flash"], d["FlashKind"] = msg, kind
	}
	s.render(w, "legal.html", d)
}

func (s *Server) deletionSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.checkCSRF(r) {
		http.Error(w, "Phiên đã hết hạn, vui lòng tải lại trang", http.StatusForbidden)
		return
	}
	if err := s.lr.DeletionRequest(r.FormValue("phone"), r.FormValue("note")); err != nil {
		s.setFlash(w, err.Error(), false)
	} else {
		s.setFlash(w, "Đã nhận yêu cầu. 3Credit sẽ xử lý và phản hồi trong thời hạn luật định.", true)
	}
	http.Redirect(w, r, "/xoa-du-lieu", http.StatusSeeOther)
}
