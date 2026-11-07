package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ---------- CMS: separate host (cms.<domain>), form login, first-run setup ----------

const (
	cmsCookie  = "3c_cms"
	cmsSession = 7 * 24 * time.Hour
)

type userKey struct{}

// actor is the logged-in CMS user, used for audit trails.
func (s *Server) actor(r *http.Request) string {
	if u, ok := r.Context().Value(userKey{}).(string); ok {
		return u
	}
	return "system"
}

func hostOnly(h string) string {
	if x, _, err := net.SplitHostPort(h); err == nil {
		h = x
	}
	return strings.ToLower(h)
}

func (s *Server) isCMSHost(r *http.Request) bool {
	return s.cfg.CMSHost != "" && hostOnly(r.Host) == s.cfg.CMSHost
}

func (s *Server) cmsURL(path string) string {
	scheme := "http"
	if strings.HasPrefix(s.cfg.BaseURL, "https://") {
		scheme = "https"
	}
	return scheme + "://" + s.cfg.CMSHost + path
}

func isCMSPath(p string) bool {
	return p == "/admin" || strings.HasPrefix(p, "/admin/") || p == "/login" || p == "/logout" || p == "/setup"
}

// route sends CMS paths to the CMS host and keeps the CMS host for CMS pages only.
func (s *Server) route(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if s.cfg.CMSHost == "" { // local / IP access: CMS served on the same host
			h.ServeHTTP(w, r)
			return
		}
		cms := s.isCMSHost(r)
		switch {
		case cms && (isCMSPath(p) || strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/uploads/") || p == "/healthz"):
			h.ServeHTTP(w, r)
		case cms:
			http.Redirect(w, r, "/admin", http.StatusFound)
		case isCMSPath(p):
			http.Redirect(w, r, s.cmsURL(r.URL.RequestURI()), http.StatusFound)
		default:
			h.ServeHTTP(w, r)
		}
	})
}

// admin guards CMS pages with the session cookie, then CSRF on POST.
func (s *Server) admin(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := ""
		if c, err := r.Cookie(cmsCookie); err == nil {
			user = s.st.CMSSessionUser(c.Value)
		}
		if user == "" {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
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
		h(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

// ---------- login throttling ----------

type limiter struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

var loginLimit = &limiter{m: map[string][]time.Time{}}

// allow reports whether key has fewer than max failures in the window.
func (l *limiter) allow(key string, max int, win time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	keep := l.m[key][:0]
	for _, t := range l.m[key] {
		if now.Sub(t) < win {
			keep = append(keep, t)
		}
	}
	l.m[key] = keep
	return len(keep) < max
}

func (l *limiter) fail(key string) {
	l.mu.Lock()
	l.m[key] = append(l.m[key], time.Now())
	l.mu.Unlock()
}

// ---------- setup code ----------

var (
	setupOnce sync.Once
	setupCode string
)

// SetupCode is needed to create the first CMS account. It is ADMIN_PASSWORD from .env
// when set, otherwise a random code printed to the server log.
func (s *Server) SetupCode() string {
	setupOnce.Do(func() {
		if s.cfg.AdminPassword != "" {
			setupCode = s.cfg.AdminPassword
			return
		}
		b := make([]byte, 5)
		rand.Read(b)
		setupCode = strings.ToUpper(hex.EncodeToString(b))
	})
	return setupCode
}

// AnnounceSetup logs how to create the first account when there is none.
func (s *Server) AnnounceSetup() {
	if n, err := s.st.CMSUserCount(); err == nil && n == 0 {
		where := "/setup"
		if s.cfg.CMSHost != "" {
			where = s.cmsURL("/setup")
		}
		if s.cfg.AdminPassword != "" {
			log.Printf("CMS chưa có tài khoản. Tạo tại %s, mã thiết lập = ADMIN_PASSWORD trong file .env", where)
		} else {
			log.Printf("CMS chưa có tài khoản. Tạo tại %s, Mã thiết lập CMS: %s", where, s.SetupCode())
		}
	}
}

// ---------- handlers ----------

func safeNext(n string) string {
	if !strings.HasPrefix(n, "/admin") || strings.HasPrefix(n, "//") {
		return "/admin"
	}
	return n
}

func (s *Server) setCMSCookie(w http.ResponseWriter, tok string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: cmsCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: strings.HasPrefix(s.cfg.BaseURL, "https://"), MaxAge: maxAge})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request, mode, msg string) {
	d := s.base(r, "Đăng nhập CMS – 3Credit")
	d["Mode"], d["Error"], d["Next"] = mode, msg, safeNext(r.FormValue("next"))
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, "cms_login.html", d)
}

func (s *Server) loginGet(w http.ResponseWriter, r *http.Request) {
	if n, _ := s.st.CMSUserCount(); n == 0 {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if c, err := r.Cookie(cmsCookie); err == nil && s.st.CMSSessionUser(c.Value) != "" {
		http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
		return
	}
	s.loginPage(w, r, "login", "")
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.checkCSRF(r) {
		s.loginPage(w, r, "login", "Phiên đã hết hạn, vui lòng thử lại")
		return
	}
	user := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	ipKey, userKey := "ip:"+s.clientIP(r), "u:"+user
	if !loginLimit.allow(ipKey, 20, 15*time.Minute) || !loginLimit.allow(userKey, 8, 15*time.Minute) {
		s.loginPage(w, r, "login", "Sai quá nhiều lần. Vui lòng thử lại sau 15 phút.")
		return
	}
	tok, err := s.st.CMSLogin(user, r.FormValue("password"), cmsSession)
	if err != nil {
		loginLimit.fail(ipKey)
		loginLimit.fail(userKey)
		s.st.Audit(user, "login_failed", "cms", user, s.clientIP(r))
		s.loginPage(w, r, "login", "Sai tên đăng nhập hoặc mật khẩu")
		return
	}
	s.st.Audit(user, "login", "cms", user, s.clientIP(r))
	s.setCMSCookie(w, tok, int(cmsSession.Seconds()))
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err == nil && s.checkCSRF(r) {
		if c, err := r.Cookie(cmsCookie); err == nil {
			s.st.CMSLogout(c.Value)
		}
	}
	s.setCMSCookie(w, "", -1)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) setupGet(w http.ResponseWriter, r *http.Request) {
	if n, _ := s.st.CMSUserCount(); n > 0 {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.loginPage(w, r, "setup", "")
}

func (s *Server) setupPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || !s.checkCSRF(r) {
		s.loginPage(w, r, "setup", "Phiên đã hết hạn, vui lòng thử lại")
		return
	}
	ipKey := "setup:" + s.clientIP(r)
	if !loginLimit.allow(ipKey, 10, 15*time.Minute) {
		s.loginPage(w, r, "setup", "Sai quá nhiều lần. Vui lòng thử lại sau 15 phút.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(r.FormValue("code"))), []byte(s.SetupCode())) != 1 {
		loginLimit.fail(ipKey)
		s.loginPage(w, r, "setup", "Mã thiết lập không đúng")
		return
	}
	pw := r.FormValue("password")
	if pw != r.FormValue("password2") {
		s.loginPage(w, r, "setup", "Hai lần nhập mật khẩu không khớp")
		return
	}
	user := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
	if err := s.st.CreateFirstCMSUser(user, pw); err != nil {
		s.loginPage(w, r, "setup", err.Error())
		return
	}
	s.st.Audit(user, "create", "cms_user", user, "first account")
	tok, err := s.st.CMSLogin(user, pw, cmsSession)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	s.setCMSCookie(w, tok, int(cmsSession.Seconds()))
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// ---------- account management ----------

func (s *Server) adminUsers(w http.ResponseWriter, r *http.Request) {
	d := s.adminData(w, r, "users", "Tài khoản CMS")
	d["Users"], _ = s.st.CMSUsers()
	s.render(w, "cms_users.html", d)
}

func (s *Server) adminUsersPost(w http.ResponseWriter, r *http.Request) {
	me := s.actor(r)
	switch r.FormValue("action") {
	case "create":
		u := strings.ToLower(strings.TrimSpace(r.FormValue("username")))
		if err := s.st.CreateCMSUser(u, r.FormValue("password")); err != nil {
			s.back(w, r, "/admin/users", err.Error(), false)
			return
		}
		s.st.Audit(me, "create", "cms_user", u, "")
		s.back(w, r, "/admin/users", "Đã tạo tài khoản "+u, true)
	case "password":
		if _, err := s.st.CMSLogin(me, r.FormValue("current"), time.Minute); err != nil {
			s.back(w, r, "/admin/users", "Mật khẩu hiện tại không đúng", false)
			return
		}
		if r.FormValue("password") != r.FormValue("password2") {
			s.back(w, r, "/admin/users", "Hai lần nhập mật khẩu mới không khớp", false)
			return
		}
		if err := s.st.SetCMSPassword(me, r.FormValue("password")); err != nil {
			s.back(w, r, "/admin/users", err.Error(), false)
			return
		}
		s.st.Audit(me, "change_password", "cms_user", me, "")
		s.setCMSCookie(w, "", -1)
		s.back(w, r, "/login", "Đã đổi mật khẩu, vui lòng đăng nhập lại", true)
	case "delete":
		u := r.FormValue("username")
		if u == me {
			s.back(w, r, "/admin/users", "Không thể tự xóa tài khoản đang đăng nhập", false)
			return
		}
		if err := s.st.DeleteCMSUser(u); err != nil {
			s.back(w, r, "/admin/users", err.Error(), false)
			return
		}
		s.st.Audit(me, "delete", "cms_user", u, "")
		s.back(w, r, "/admin/users", "Đã xóa tài khoản "+u, true)
	default:
		s.back(w, r, "/admin/users", "Thao tác không hợp lệ", false)
	}
}
