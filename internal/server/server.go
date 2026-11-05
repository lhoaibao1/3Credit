// Package server wires HTTP routes for the customer site, the admin
// dashboard, tracking redirects and postbacks.
package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/threecredit/platform/internal/leads"
	"github.com/threecredit/platform/internal/matching"
	"github.com/threecredit/platform/internal/model"
	"github.com/threecredit/platform/internal/store"
	"github.com/threecredit/platform/internal/tracking"
	"github.com/threecredit/platform/web"
)

type Config struct {
	BaseURL       string
	DataDir       string
	SessionSecret string
	AdminUser     string
	AdminPassword string
	TrustCF       bool // behind Cloudflare: read the visitor IP from CF-Connecting-IP
}

type Server struct {
	cfg Config
	st  *store.Store
	tr  *tracking.Tracker
	lr  *leads.Router
	tpl *template.Template
	mux *http.ServeMux
	now func() time.Time
}

func New(cfg Config, st *store.Store, tr *tracking.Tracker, lr *leads.Router) (*Server, error) {
	s := &Server{cfg: cfg, st: st, tr: tr, lr: lr, mux: http.NewServeMux(), now: time.Now}
	t, err := template.New("").Funcs(s.funcs()).ParseFS(web.FS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s.tpl = t
	s.routes()
	return s, nil
}

func (s *Server) Handler() http.Handler { return s.secure(s.session(s.mux)) }

func (s *Server) routes() {
	static, _ := fs.Sub(web.FS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", cacheStatic(http.FileServer(http.FS(static)))))
	s.mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", uploadsHeaders(http.FileServer(http.Dir(filepath.Join(s.cfg.DataDir, "uploads"))))))
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	s.mux.HandleFunc("GET /{$}", s.home)
	s.mux.HandleFunc("GET /api/match", s.apiMatch)
	s.mux.HandleFunc("GET /san-pham/{id}", s.detail)
	s.mux.HandleFunc("GET /so-sanh", s.compare)
	s.mux.HandleFunc("GET /go/{id}", s.goClick)
	s.mux.HandleFunc("POST /dang-ky/{id}", s.leadSubmit)
	s.mux.HandleFunc("GET /api/postback/{network}", s.postback)
	s.mux.HandleFunc("POST /api/postback/{network}", s.postback)

	s.mux.HandleFunc("GET /kien-thuc", s.newsList)
	s.mux.HandleFunc("GET /kien-thuc/{slug}", s.newsDetail)
	s.mux.HandleFunc("POST /tu-van", s.inquirySubmit)
	for path := range legalPages {
		s.mux.HandleFunc("GET "+path, s.legal)
	}
	s.mux.HandleFunc("POST /xoa-du-lieu", s.deletionSubmit)
	s.mux.HandleFunc("GET /gioi-thieu", s.about)

	a := func(h http.HandlerFunc) http.Handler { return s.admin(h) }
	s.mux.Handle("GET /admin/inquiries", a(s.adminInquiries))
	s.mux.Handle("GET /admin/inquiries.csv", a(s.adminInquiriesCSV))
	s.mux.Handle("POST /admin/inquiries/{id}", a(s.adminInquiryUpdate))
	s.mux.Handle("GET /admin/posts", a(s.adminPosts))
	s.mux.Handle("GET /admin/posts/{id}", a(s.adminPostEdit))
	s.mux.Handle("POST /admin/posts/{id}", a(s.adminPostSave))
	s.mux.Handle("GET /admin/settings", a(s.adminSettings))
	s.mux.Handle("POST /admin/settings", a(s.adminSettingsSave))
	s.mux.Handle("GET /admin/deletions", a(s.adminDeletions))
	s.mux.Handle("POST /admin/deletions/{id}", a(s.adminDeletionDo))
	s.mux.Handle("POST /admin/campaigns/{id}/status", a(s.adminCampaignStatus))
	s.mux.Handle("POST /admin/products/new", a(s.adminProductNew))
	s.mux.Handle("POST /admin/partners/new", a(s.adminPartnerNew))
	s.mux.Handle("GET /admin", a(s.adminOverview))
	s.mux.Handle("GET /admin/campaigns", a(s.adminCampaigns))
	s.mux.Handle("POST /admin/campaigns/new", a(s.adminCampaignNew))
	s.mux.Handle("GET /admin/campaigns/{id}", a(s.adminCampaignEdit))
	s.mux.Handle("POST /admin/campaigns/{id}", a(s.adminCampaignSave))
	s.mux.Handle("GET /admin/partners", a(s.adminPartners))
	s.mux.Handle("POST /admin/partners/{id}", a(s.adminPartnerSave))
	s.mux.Handle("GET /admin/policies/{product}", a(s.adminPolicy))
	s.mux.Handle("POST /admin/policies/{product}", a(s.adminPolicySave))
	s.mux.Handle("GET /admin/segments", a(s.adminSegments))
	s.mux.Handle("POST /admin/segments", a(s.adminSegmentSave))
	s.mux.Handle("GET /admin/matching", a(s.adminMatching))
	s.mux.Handle("GET /admin/tracking", a(s.adminTracking))
	s.mux.Handle("GET /admin/leads", a(s.adminLeads))
	s.mux.Handle("GET /admin/audit", a(s.adminAudit))
}

// ---------- middleware ----------

const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

func (s *Server) secure(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", csp)
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(s.cfg.BaseURL, "https://") {
			hd.Set("Strict-Transport-Security", "max-age=31536000")
		}
		h.ServeHTTP(w, r)
	})
}

func cacheStatic(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		h.ServeHTTP(w, r)
	})
}

func uploadsHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
		w.Header().Set("Cache-Control", "public, max-age=600")
		h.ServeHTTP(w, r)
	})
}

type ctxKey struct{}

const sidCookie = "3c_sid"

func (s *Server) session(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(sidCookie); err != nil || len(c.Value) != 32 {
			b := make([]byte, 16)
			rand.Read(b)
			v := hex.EncodeToString(b)
			http.SetCookie(w, &http.Cookie{Name: sidCookie, Value: v, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
				Secure: strings.HasPrefix(s.cfg.BaseURL, "https://"), MaxAge: 60 * 60 * 24 * 30})
			r.AddCookie(&http.Cookie{Name: sidCookie, Value: v})
		}
		h.ServeHTTP(w, r)
	})
}

func sid(r *http.Request) string {
	if c, err := r.Cookie(sidCookie); err == nil {
		return c.Value
	}
	return ""
}

func (s *Server) csrf(r *http.Request) string {
	m := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	m.Write([]byte("csrf:" + sid(r)))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Server) checkCSRF(r *http.Request) bool {
	return hmac.Equal([]byte(r.FormValue("csrf")), []byte(s.csrf(r)))
}

func (s *Server) admin(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AdminUser == "" || s.cfg.AdminPassword == "" {
			http.Error(w, "Trang quản trị bị tắt: đặt ADMIN_USER và ADMIN_PASSWORD.", http.StatusServiceUnavailable)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.cfg.AdminUser)) != 1 || subtle.ConstantTimeCompare([]byte(p), []byte(s.cfg.AdminPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="3Credit admin", charset="UTF-8"`)
			http.Error(w, "Cần đăng nhập", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodPost {
			if err := r.ParseMultipartForm(2 << 20); err != nil && err != http.ErrNotMultipart {
				http.Error(w, "form quá lớn", http.StatusBadRequest)
				return
			}
			if !s.checkCSRF(r) {
				http.Error(w, "CSRF token không hợp lệ, tải lại trang", http.StatusForbidden)
				return
			}
		}
		h(w, r)
	})
}

// ---------- rendering ----------

func (s *Server) render(w http.ResponseWriter, name string, data map[string]any) {
	var buf bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&buf, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
		http.Error(w, "Lỗi hiển thị", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

type cardData struct {
	R matching.Result
	Q template.URL
}

func (s *Server) funcs() template.FuncMap {
	return template.FuncMap{
		"assetv": func() string { return assetVersion },
		"eco":    func() []store.EcoMember { return store.Ecosystem },
		"initials": func(n string) string {
			var b strings.Builder
			for _, w := range strings.Fields(n) {
				r := []rune(w)
				if len(r) > 0 && (unicode.IsLetter(r[0]) || unicode.IsDigit(r[0])) {
					b.WriteRune(unicode.ToUpper(r[0]))
				}
				if b.Len() >= 3 {
					break
				}
			}
			return b.String()
		},
		"money":    func(v float64) string { return moneyText(v) },
		"millions": func(v float64) int { return int(math.Round(v / 1e6)) },
		"pct":      func(f float64) int { return int(math.Round(f * 100)) },
		"barw":     func(r matching.Result) int { return int(math.Round(float64(r.Fit) * r.Confidence)) },
		"join":     strings.Join,
		"first": func(xs []string, n int) []string {
			if len(xs) > n {
				return xs[:n]
			}
			return xs
		},
		"sub":      func(a, b int) int { return a - b },
		"card":     func(r matching.Result, q template.URL) cardData { return cardData{R: r, Q: q} },
		"verified": func(o model.Offer) int { return verifiedPct(o) },
		"vnd":      func(v float64) string { return vnd(v) },
		"dtlocal": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Local().Format("2006-01-02T15:04")
		},
		"vi":   Label,
		"seq":  func(n int) []int { return make([]int, n) },
		"pair": func(a, b string) struct{ A, B string } { return struct{ A, B string }{a, b} },
		"has": func(xs []string, v string) bool {
			for _, x := range xs {
				if x == v {
					return true
				}
			}
			return false
		},
	}
}

func moneyText(v float64) string {
	if v >= 1e9 {
		return strings.TrimSuffix(strconv.FormatFloat(v/1e9, 'f', 1, 64), ".0") + " tỷ"
	}
	return strconv.FormatFloat(v/1e6, 'f', 0, 64) + " triệu"
}

func vnd(v float64) string {
	s := strconv.FormatInt(int64(math.Round(v)), 10)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, '.')
		}
		out = append(out, s[i])
	}
	return string(out) + "đ"
}

func verifiedPct(o model.Offer) int {
	fs := []*model.Fact{o.Policy.Info.Rate, o.Policy.Info.Amount, o.Policy.Info.Term, o.Policy.Info.Fee, o.Policy.Info.Docs, o.Policy.Info.Speed}
	n := 0
	for _, f := range fs {
		if f != nil && f.Conf.Verified() {
			n++
		}
	}
	return n * 100 / len(fs)
}

func (s *Server) base(r *http.Request, title string) map[string]any {
	return map[string]any{"Title": title, "CSRF": s.csrf(r), "S": s.st.Settings(), "Path": r.URL.Path}
}

func (s *Server) logoDir() string { return filepath.Join(s.cfg.DataDir, "uploads", "partners") }

func ensureDir(p string) error { return os.MkdirAll(p, 0o755) }

func q(v url.Values) template.URL { return template.URL(v.Encode()) }

func errText(err error) string { return fmt.Sprint(err) }

// ---------- flash messages (signed cookie, never from the URL) ----------

const flashCookie = "3c_flash"

func (s *Server) sign(v string) string {
	m := hmac.New(sha256.New, []byte(s.cfg.SessionSecret))
	m.Write([]byte("flash:" + v))
	return hex.EncodeToString(m.Sum(nil))[:24]
}

func (s *Server) setFlash(w http.ResponseWriter, msg string, ok bool) {
	kind := "err"
	if ok {
		kind = "ok"
	}
	v := url.QueryEscape(kind + "|" + msg)
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: v + "." + s.sign(v), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 60})
}

func (s *Server) takeFlash(w http.ResponseWriter, r *http.Request) (string, string) {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return "", ""
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: "", Path: "/", MaxAge: -1})
	i := strings.LastIndex(c.Value, ".")
	if i < 0 || !hmac.Equal([]byte(c.Value[i+1:]), []byte(s.sign(c.Value[:i]))) {
		return "", ""
	}
	raw, err := url.QueryUnescape(c.Value[:i])
	if err != nil {
		return "", ""
	}
	kind, msg, _ := strings.Cut(raw, "|")
	return msg, kind
}

// assetVersion changes whenever any static file changes, so CDN/browser caches
// never serve an old stylesheet with a new page.
var assetVersion = func() string {
	h := sha256.New()
	fs.WalkDir(web.FS, "static", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := web.FS.ReadFile(p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:10]
}()
